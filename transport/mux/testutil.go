package mux

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"golang.org/x/sync/semaphore"
)

var onConnectionNoOp = func(map[string]session.ManagedMuxSession) {}

type pipedMuxManagers struct {
	serverConnCh chan net.Conn
	serverMM     MultiMuxManager
	serverCancel context.CancelFunc
	serverEvents chan connectionEvent
	clientConnCh chan net.Conn
	clientMM     MultiMuxManager
	clientCancel context.CancelFunc
	clientEvents chan connectionEvent
}

func (p *pipedMuxManagers) assertClientAndServerEvents(t *testing.T, n int, fn func(*testing.T, connectionEvent), msg string) {
	for range n {
		serverEvent := requireCh(t, p.serverEvents, 100*time.Millisecond, msg)
		fn(t, serverEvent)
		clientEvent := requireCh(t, p.clientEvents, 100*time.Millisecond, msg)
		fn(t, clientEvent)
	}
}
func (p *pipedMuxManagers) assertNoConnectionEvents(t *testing.T, msg string) {
	requireNoCh(t, p.clientEvents, 100*time.Millisecond, msg)
	requireNoCh(t, p.serverEvents, 100*time.Millisecond, msg)
}

func eventIsOpened(t *testing.T, e connectionEvent) {
	require.Equal(t, "opened", e.eventType, "Wrong event type %v", e.eventType)
}

func eventIsClosed(t *testing.T, e connectionEvent) {
	require.Equal(t, "closed", e.eventType, "Wrong event type %v", e.eventType)
}

func buildMuxesOnPipes(t *testing.T, logger log.Logger, numConns int64, onServerChange OnConnectionListUpdate, onClientChange OnConnectionListUpdate) (muxesOnPipes pipedMuxManagers) {
	muxesOnPipes.serverConnCh, muxesOnPipes.clientConnCh = make(chan net.Conn, numConns), make(chan net.Conn, numConns)
	for range numConns {
		serverConn, clientConn := net.Pipe()
		muxesOnPipes.serverConnCh <- serverConn
		muxesOnPipes.clientConnCh <- clientConn
	}
	serverProvider := func(ctx context.Context) connProvider { return &channelConnProvider{ctx, muxesOnPipes.serverConnCh} }
	clientProvider := func(ctx context.Context) connProvider { return &channelConnProvider{ctx, muxesOnPipes.clientConnCh} }
	serverConnProvider := buildTestMuxProviderBuilder("serverProvider", serverProvider, yamux.Server, numConns, logger)
	clientConnProvider := buildTestMuxProviderBuilder("clientProvider", clientProvider, yamux.Client, numConns, logger)
	muxesOnPipes.serverMM, muxesOnPipes.serverEvents, muxesOnPipes.serverCancel = buildMuxManager(t, "serverMux", serverConnProvider, logger, onServerChange)
	muxesOnPipes.clientMM, muxesOnPipes.clientEvents, muxesOnPipes.clientCancel = buildMuxManager(t, "clientMux", clientConnProvider, logger, onClientChange)
	return
}

type channelConnProvider struct {
	lifetime    context.Context
	connections <-chan net.Conn
}

func (p *channelConnProvider) NewConnection() (net.Conn, error) {
	select {
	case <-p.lifetime.Done():
		return nil, p.lifetime.Err()
	case conn := <-p.connections:
		return conn, nil
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("timed out")
	}
}
func (p *channelConnProvider) CloseCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (p *channelConnProvider) Address() string {
	return "Pipe"
}

type connectionEvent struct {
	id        string
	session   *yamux.Session
	eventType string
}

func buildTestMuxProviderBuilder(name string,
	buildProvider func(lifetime context.Context) connProvider,
	yamuxFn func(io.ReadWriteCloser, *yamux.Config) (*yamux.Session, error),
	numConns int64,
	logger log.Logger,
) MuxProviderBuilder {
	return func(cb AddNewMux, lifetime context.Context) (MuxProvider, error) {
		provider := &muxProvider{
			name:         fmt.Sprintf("provider-%s", name),
			connProvider: buildProvider(lifetime),
			sessionFn: func(conn net.Conn) (*yamux.Session, error) {
				yamuxSession, err := yamuxFn(conn, nil)
				go func() {
					<-lifetime.Done()
					_ = yamuxSession.Close()
				}()
				return yamuxSession, err
			},
			addNewMux:    cb,
			muxPermits:   semaphore.NewWeighted(numConns),
			metricLabels: []string{"test", "mux", "manager"},
			logger:       logger,
			startOnce:    sync.Once{},
			hasStarted:   atomic.Bool{},
			lifetime:     lifetime,
			hasCleanedUp: channel.NewShutdownOnce(),
		}
		return provider, nil
	}
}

func buildMuxManager(t *testing.T,
	name string,
	muxProviderBuilder MuxProviderBuilder,
	logger log.Logger,
	connectionListObservers ...OnConnectionListUpdate,
) (MultiMuxManager, chan connectionEvent, context.CancelFunc) {
	eventsCh := make(chan connectionEvent, 100)
	ctx, cancel := context.WithCancel(context.Background())
	mgr, err := NewCustomMultiMuxManager(ctx,
		name,
		muxProviderBuilder,
		[]session.StartManagedComponentFn{eventsComponent(eventsCh)},
		connectionListObservers,
		logger)
	require.NoError(t, err)
	return mgr, eventsCh, cancel
}

func eventsComponent(eventsCh chan connectionEvent) session.StartManagedComponentFn {
	return func(id string, session *yamux.Session, shutdown channel.ShutdownOnce) {
		// Deliberately not just closing the channels so that they can be reused by multiple threads
		go func() {
			eventsCh <- connectionEvent{id, session, "opened"}
			for {
				select {
				case <-shutdown.Channel():
					eventsCh <- connectionEvent{id, session, "closed"}
					return
				}
			}
		}()
	}
}

func requireNoCh[T any](t *testing.T, ch <-chan T, timeout time.Duration, message string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(message)
	case <-time.After(timeout):
	}
}

func requireCh[T any](t *testing.T, ch chan T, timeout time.Duration, message string) T {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(timeout):
		t.Fatal(message)
		// Never returned, but Go needs this
		var empty T
		return empty
	}
}
