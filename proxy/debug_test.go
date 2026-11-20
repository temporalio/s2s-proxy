package proxy

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"github.com/temporalio/s2s-proxy/transport/mux"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

func TestGetMuxConnectionsInfo(t *testing.T) {
	pooledClient := &debugSnapshotClient{snapshot: grpcutil.PoolSnapshot{
		General:     grpcutil.PoolStatus{Target: 1, Assigned: 1, Connected: 1},
		Replication: grpcutil.PoolStatus{Target: 2, Assigned: 1},
		Assignments: map[string]string{
			"a": grpcutil.GeneralPoolName,
			"b": grpcutil.ReplicationPoolName,
		},
	}}
	muxServer := debugMuxManager{
		name:    "remote",
		address: "127.0.0.1:7233",
		sessions: map[string]session.ManagedMuxSession{
			"b": debugMuxSession{
				remote: debugAddr("remote-b"),
				info:   &session.MuxSessionInfo{State: session.Error},
				closed: true,
			},
			"a": debugMuxSession{
				local:  debugAddr("local-a"),
				remote: debugAddr("remote-a"),
				info:   &session.MuxSessionInfo{State: session.Connected},
			},
		},
	}

	for _, tt := range []struct {
		name      string
		server    contextAwareServer
		client    closableClientConn
		direction string
		want      []MuxConnectionsDebugInfo
	}{
		{
			name:   "non mux server",
			server: debugNonMuxServer{},
			client: debugClient{},
		},
		{
			name:      "pool snapshot and sessions",
			server:    muxServer,
			client:    pooledClient,
			direction: "outbound",
			want: []MuxConnectionsDebugInfo{{
				ConnectionName: "remote",
				Direction:      "outbound",
				Address:        "127.0.0.1:7233",
				Connections: []MuxConnectionInfo{
					{ID: "a", Pool: grpcutil.GeneralPoolName, LocalAddr: "local-a", RemoteAddr: "remote-a", State: mux.SessionStateConnected},
					{ID: "b", Pool: grpcutil.ReplicationPoolName, RemoteAddr: "remote-b", State: mux.SessionStateErrored, IsClosed: true},
				},
				ConnectionCount: 2,
				Pools: map[string]MuxPoolDebugInfo{
					grpcutil.GeneralPoolName:     {Target: 1, Assigned: 1, Connected: 1},
					grpcutil.ReplicationPoolName: {Target: 2, Assigned: 1},
				},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, getMuxConnectionsInfo(tt.server, tt.client, tt.direction))
		})
	}

	require.Equal(t, 1, pooledClient.calls, "debug output should use one consistent pool snapshot")
}

type debugMuxManager struct {
	mux.MultiMuxManager
	name     string
	address  string
	sessions map[string]session.ManagedMuxSession
}

func (m debugMuxManager) Name() string    { return m.name }
func (m debugMuxManager) Address() string { return m.address }
func (m debugMuxManager) GetMuxConnections() map[string]session.ManagedMuxSession {
	return m.sessions
}

type debugNonMuxServer struct{ contextAwareServer }

type debugClient struct{ closableClientConn }

type debugSnapshotClient struct {
	closableClientConn
	snapshot grpcutil.PoolSnapshot
	calls    int
}

func (c *debugSnapshotClient) Snapshot() grpcutil.PoolSnapshot {
	c.calls++
	return c.snapshot
}

type debugMuxSession struct {
	session.ManagedMuxSession
	local  net.Addr
	remote net.Addr
	info   *session.MuxSessionInfo
	closed bool
}

func (s debugMuxSession) GetConnectionInfo() (net.Addr, net.Addr) { return s.local, s.remote }
func (s debugMuxSession) State() *session.MuxSessionInfo          { return s.info }
func (s debugMuxSession) IsClosed() bool                          { return s.closed }

type debugAddr string

func (debugAddr) Network() string  { return "test" }
func (a debugAddr) String() string { return string(a) }
