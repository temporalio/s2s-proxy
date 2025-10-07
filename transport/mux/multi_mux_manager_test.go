package mux

import (
	"maps"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/endtoendtest/proxyassert"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

func init() {
	_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "error")
}

func TestMultiMuxManager(t *testing.T) {
	logger := log.NewTestLogger()

	serverConns := &atomic.Pointer[map[string]session.ManagedMuxSession]{}
	clientConns := &atomic.Pointer[map[string]session.ManagedMuxSession]{}
	muxesOnPipes := buildMuxesOnPipes(t, logger, 1,
		func(sessions map[string]session.ManagedMuxSession) {
			clone := maps.Clone(sessions)
			serverConns.Store(&clone)
		}, func(sessions map[string]session.ManagedMuxSession) {
			clone := maps.Clone(sessions)
			clientConns.Store(&clone)
		})

	// Avoid the MuxManager's Start(), which assumes we're using TCP
	muxesOnPipes.serverMM.Start()
	muxesOnPipes.clientMM.Start()

	clientEvent := proxyassert.RequireCh(t, muxesOnPipes.clientEvents, 2*time.Second, "should have seen a connection from the clientProvider")
	require.Equal(t, "opened", clientEvent.eventType)
	clientSession := clientEvent.session
	serverEvent := proxyassert.RequireCh(t, muxesOnPipes.serverEvents, 2*time.Second, "should have seen a connection from the serverProvider")
	require.Equal(t, "opened", clientEvent.eventType)
	serverSession := serverEvent.session

	// Close connections. We should see both sides fire disconnectFn
	for _, v := range *clientConns.Load() {
		v.Close()
	}
	clientEvent = proxyassert.RequireCh(t, muxesOnPipes.clientEvents, 2*time.Second, "Client connection failed to disconnect!\nclientMux:%s", muxesOnPipes.clientMM.Describe())
	require.Equal(t, "closed", clientEvent.eventType)
	require.Same(t, clientSession, clientEvent.session)
	for _, v := range *serverConns.Load() {
		v.Close()
	}
	serverEvent = proxyassert.RequireCh(t, muxesOnPipes.serverEvents, 2*time.Second, "Server connection failed to disconnect")
	require.Equal(t, "closed", serverEvent.eventType)
	require.Same(t, serverSession, serverEvent.session)

	assert.True(t, clientSession.IsClosed(), "clientSession should be closed")
	assert.True(t, serverSession.IsClosed(), "serverSession should be closed")

	clientConn, serverConn := net.Pipe()
	muxesOnPipes.clientConnCh <- clientConn
	muxesOnPipes.serverConnCh <- serverConn

	clientReconn := proxyassert.RequireCh(t, muxesOnPipes.clientEvents, 2*time.Second, "should have seen a new connection from the clientProvider")
	serverReconn := proxyassert.RequireCh(t, muxesOnPipes.serverEvents, 2*time.Second, "should have seen a new connection from the clientProvider")
	require.NotSame(t, clientSession, clientReconn.session, "Should be a new client session")
	require.NotSame(t, serverSession, serverReconn.session, "Should be a new server session")
	muxesOnPipes.clientCancel()
	muxesOnPipes.serverCancel()
}

func TestMultiMuxManager_ManyConnections(t *testing.T) {
	logger := log.NewTestLogger()
	serverConnsObservations, clientConnsObservations := make(chan int, 100), make(chan int, 100)
	muxesOnPipes := buildMuxesOnPipes(t, logger, 10,
		func(sessions map[string]session.ManagedMuxSession) {
			serverConnsObservations <- len(sessions)
		}, func(sessions map[string]session.ManagedMuxSession) {
			clientConnsObservations <- len(sessions)
		})
	proxyassert.RequireNoCh(t, muxesOnPipes.serverEvents, 20*time.Millisecond, "Nothing should happen yet, no MuxMgrs have started")
	proxyassert.RequireNoCh(t, muxesOnPipes.clientEvents, 20*time.Millisecond, "Nothing should happen yet, no MuxMgrs have started")
	muxesOnPipes.serverMM.Start()
	proxyassert.RequireNoCh(t, muxesOnPipes.serverEvents, 20*time.Millisecond, "Nothing should happen yet, client MuxMgr hasn't started")
	proxyassert.RequireNoCh(t, muxesOnPipes.clientEvents, 20*time.Millisecond, "Nothing should happen yet, client MuxMgr hasn't started")
	muxesOnPipes.clientMM.Start()
	muxesOnPipes.assertClientAndServerEvents(t, 10, eventIsOpened, "Connections should be opened")
	for i := range 10 {
		n := proxyassert.RequireCh(t, serverConnsObservations, 5*time.Millisecond, "Should see a serverConn observation for every connection")
		require.Equal(t, 1+i, n)
		n = proxyassert.RequireCh(t, clientConnsObservations, 5*time.Millisecond, "Should see a clientConn observation for every connection")
		require.Equal(t, 1+i, n)
	}
	muxesOnPipes.clientCancel()
	muxesOnPipes.serverCancel()
	muxesOnPipes.assertClientAndServerEvents(t, 10, eventIsClosed, "Connections should be opened")
	for i := range 10 {
		n := proxyassert.RequireCh(t, serverConnsObservations, 5*time.Millisecond, "Should see a serverConn observation for every connection")
		require.Equal(t, 9-i, n)
		n = proxyassert.RequireCh(t, clientConnsObservations, 5*time.Millisecond, "Should see a clientConn observation for every connection")
		require.Equal(t, 9-i, n)
	}
}

func TestMultiMuxManager_ShutDown(t *testing.T) {
	logger := log.NewTestLogger()
	muxesOnPipes := buildMuxesOnPipes(t, logger, 10, onConnectionNoOp, onConnectionNoOp)
	muxesOnPipes.serverMM.Start()
	muxesOnPipes.clientMM.Start()
	muxesOnPipes.assertClientAndServerEvents(t, 10, eventIsOpened, "Should see opening events")

	muxesOnPipes.assertNoConnectionEvents(t, "In steady state, no connections should happen")

	muxesOnPipes.clientCancel()
	muxesOnPipes.serverCancel()
	muxesOnPipes.assertClientAndServerEvents(t, 10, eventIsClosed, "Should see closed events")
}

func TestMultiMuxManager_Reconnection(t *testing.T) {
	logger := log.NewTestLogger()
	muxesOnPipes := buildMuxesOnPipes(t, logger, 10, onConnectionNoOp, onConnectionNoOp)
	muxesOnPipes.serverMM.Start()
	muxesOnPipes.clientMM.Start()
	muxesOnPipes.assertClientAndServerEvents(t, 10, eventIsOpened, "Should see opened events")
	for range 10 {
		serverConn, clientConn := net.Pipe()
		muxesOnPipes.serverConnCh <- serverConn
		muxesOnPipes.clientConnCh <- clientConn
	}
	muxesOnPipes.assertNoConnectionEvents(t, "Existing connections should be saturated, no additional connections should happen")

	muxesOnPipes.clientCancel()
	muxesOnPipes.serverCancel()
	muxesOnPipes.assertClientAndServerEvents(t, 10, eventIsClosed, "Should see closed events")
	muxesOnPipes.assertNoConnectionEvents(t, "Existing connections should be saturated, no additional connections should happen")
}
