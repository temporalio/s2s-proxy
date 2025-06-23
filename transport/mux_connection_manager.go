package transport

import (
	"crypto/tls"
	"fmt"
	"github.com/temporalio/s2s-proxy/metrics"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type status int32

const (
	statusInitialized status = iota
	statusStarted
	statusStopped
)

const (
	throttleRetryInitialInterval = time.Second
	throttleRetryMaxInterval     = 30 * time.Second
)

var (
	retryPolicy = backoff.NewExponentialRetryPolicy(throttleRetryInitialInterval).
		WithBackoffCoefficient(1.5).
		WithMaximumInterval(throttleRetryMaxInterval)
)

type (
	muxConnectMananger struct {
		config       config.MuxTransportConfig
		muxTransport *muxTransportImpl
		shutdownCh   chan struct{}
		connectedCh  chan struct{} // if closed, means transport is connected.
		mu           sync.Mutex
		wg           sync.WaitGroup
		logger       log.Logger
		status       atomic.Int32
	}
)

func newMuxConnectManager(cfg config.MuxTransportConfig, logger log.Logger) *muxConnectMananger {
	cm := &muxConnectMananger{
		config: cfg,
		logger: log.With(logger, tag.NewStringTag("Name", cfg.Name), tag.NewStringTag("Mode", string(cfg.Mode))),
	}

	cm.status.Store(int32(statusInitialized))
	return cm
}

func (m *muxConnectMananger) open() (MuxTransport, error) {
	if !m.isStarted() {
		return nil, fmt.Errorf("Connection manager is not running.")
	}

	// Wait for transport to be connected
	// We use a lock here to prevent from reading stale muxTransport data:
	//   t0: (this) open is wait for <-connectedCh
	//   t1: (loop) muxTransport is set and connectedCh is closed
	//   t2: (loop) muxTransport is closed
	//   t3: (loop) muxTransport is set to nil
	//   t4: (this) read muxTransport = nil
	//
	// Holding a lock when reading from <-connectedCh should not cause deadlock.
	// Deadlock can happen if:
	//  1. (this) has the lock
	//  2. (loop) tries to acquire the lock while <-connectedCh is open
	// #2 can't happen because connectedCh is closed before (loop) tries to acquire the lock (see waitForReconnect)

	// TODO: Add some timeout here in case connection can't be fulfilled.
	var muxTransport *muxTransportImpl
	m.mu.Lock()
	select {
	case <-m.shutdownCh:
		m.mu.Unlock()
		return nil, fmt.Errorf("Connection manager is not running.")

	case <-m.connectedCh:
		muxTransport = m.muxTransport
	}
	m.mu.Unlock()

	if muxTransport.session.IsClosed() {
		return nil, fmt.Errorf("Session is closed")
	}
	return muxTransport, nil
}

func (m *muxConnectMananger) isShuttingDown() bool {
	select {
	case <-m.shutdownCh:
		return true
	default:
		return false
	}
}

func (m *muxConnectMananger) serverLoop(setting config.TCPServerSetting) error {
	var tlsConfig *tls.Config
	var err error
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err = encryption.GetServerTLSConfig(tlsCfg)
		if err != nil {
			return err
		}
	}

	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return err
	}

	m.wg.Add(1)
	go func() {
		defer func() {
			listener.Close()
			m.wg.Done()
		}()

		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Debug("listener accepting new connection")
				// Accept a TCP connection
				server, err := listener.Accept()
				if err != nil {
					if m.isShuttingDown() {
						return
					}

					m.logger.Fatal("listener.Accept failed", tag.Error(err))
				}

				var conn net.Conn

				if tlsConfig != nil {
					conn = tls.Server(server, tlsConfig)
				} else {
					conn = server
				}

				m.logger.Info("Accept new connection", tag.NewStringTag("remoteAddr", conn.RemoteAddr().String()))

				session, err := yamux.Server(conn, nil)
				go observeYamuxSession(session, m.config)
				if err != nil {
					m.logger.Fatal("yamux.Server failed", tag.Error(err))
				}

				m.muxTransport = newMuxTransport(conn, session)
				m.waitForReconnect()
			}
		}
	}()

	go func() {
		// wait for shutdown
		<-m.shutdownCh
		listener.Close() // this will cause listener.Accept to fail.
	}()

	return nil
}

func (m *muxConnectMananger) clientLoop(setting config.TCPClientSetting) error {
	var tlsConfig *tls.Config
	var err error
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return err
		}
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Info("mux client dialing server")

				var client net.Conn
				op := func() error {
					var err error
					client, err = net.DialTimeout("tcp", setting.ServerAddress, 5*time.Second)
					return err
				}

				if err := backoff.ThrottleRetry(op, retryPolicy, func(err error) bool {
					return !m.isShuttingDown()
				}); err != nil {
					m.logger.Error("mux client failed to dial", tag.Error(err))
					return
				}

				var conn net.Conn
				if tlsConfig != nil {
					conn = tls.Client(client, tlsConfig)
				} else {
					conn = client
				}

				session, err := yamux.Client(conn, nil)
				go observeYamuxSession(session, m.config)
				if err != nil {
					m.logger.Fatal("yamux.Client failed", tag.Error(err))
				}

				m.muxTransport = newMuxTransport(conn, session)
				m.waitForReconnect()
			}
		}
	}()

	return nil
}

// observeYamuxSession creates a goroutine that pings the provided yamux session repeatedly and gathers its two
// metrics: Whether the server is alive and how many streams it has open.
func observeYamuxSession(session *yamux.Session, config config.MuxTransportConfig) {
	if session == nil {
		// If we got a null session, we can't even generate tags to report
		return
	}
	defer func() {
		// This is an async monitor. Don't let it crash the rest of the program if there's a problem
		recover()
	}()
	labels := []string{session.LocalAddr().String(),
		session.RemoteAddr().String(),
		string(config.Mode),
		config.Name,
	}
	var serverActive float64 = 1
	// It's possible the server was never opened, make sure we emit a 0 in that case
	if session.IsClosed() {
		metrics.MuxSessionOpen.WithLabelValues(labels...).Set(0)
		metrics.MuxStreamsActive.WithLabelValues(labels...).Set(float64(0))
		return
	}
	ticker := time.NewTicker(time.Minute)
	for !session.IsClosed() {
		// Prometheus gauges are cheap, but Session.NumStreams() takes a mutex in the session! Only check once per minute
		// to minimize overhead
		select {
		case <-session.CloseChan():
			serverActive = 0
		case <-ticker.C:
			// wake up so we can report NumStreams
		}
		metrics.MuxSessionOpen.WithLabelValues(labels...).Set(serverActive)
		metrics.MuxStreamsActive.WithLabelValues(labels...).Set(float64(session.NumStreams()))
	}
}

func (m *muxConnectMananger) start() error {
	if !m.status.CompareAndSwap(
		int32(statusInitialized),
		int32(statusStarted),
	) {
		return fmt.Errorf("Connection manager can't be started. status: %d", m.getStatus())
	}

	m.shutdownCh = make(chan struct{})
	m.connectedCh = make(chan struct{})

	switch m.config.Mode {
	case config.ClientMode:
		m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Client))
		if err := m.clientLoop(m.config.Client); err != nil {
			return err
		}
	case config.ServerMode:
		m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Server))
		if err := m.serverLoop(m.config.Server); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s.", m.config.Name, m.config.Mode)
	}

	return nil
}

func (m *muxConnectMananger) isStarted() bool {
	return m.getStatus() == statusStarted
}

func (m *muxConnectMananger) getStatus() status {
	return status(m.status.Load())
}

func (m *muxConnectMananger) stop() {
	if !m.status.CompareAndSwap(
		int32(statusStarted),
		int32(statusStopped),
	) {
		return
	}

	close(m.shutdownCh)
	m.wg.Wait()
	m.logger.Info("Connection manager stopped")
}

func (m *muxConnectMananger) waitForReconnect() {
	// Notify transport is connected
	close(m.connectedCh)

	m.logger.Debug("connected")

	select {
	case <-m.shutdownCh:
		m.muxTransport.closeSession()
	case <-m.muxTransport.session.CloseChan():
		m.muxTransport.closeSession()
	}

	m.logger.Debug("disconnected")

	// Initialize connectedCh and muxTransport. Invariant: connectCh is closed.
	m.mu.Lock()
	m.connectedCh = make(chan struct{})
	muxTransport := m.muxTransport
	m.muxTransport = nil
	m.mu.Unlock()

	// Notify transport is closed after we reset connectedCh to avoid the case that
	// the caller of Open receives an already closed transport.
	close(muxTransport.closeCh)
}
