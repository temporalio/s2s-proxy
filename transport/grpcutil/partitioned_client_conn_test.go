package grpcutil

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

func TestReconcilePoolAssignments(t *testing.T) {
	tests := []struct {
		name      string
		previous  map[string]trafficClass
		available []string
		targets   poolTargets
		want      map[string]trafficClass
	}{
		{name: "empty", targets: poolTargets{general: 1, replication: 1}, want: map[string]trafficClass{}},
		{
			name:      "unordered initial sessions are deterministic",
			available: []string{"2", "0", "1"},
			targets:   poolTargets{general: 1, replication: 2},
			want: map[string]trafficClass{
				"0": trafficGeneral, "1": trafficReplication, "2": trafficReplication,
			},
		},
		{
			name: "surviving assignments are retained and replication is refilled",
			previous: map[string]trafficClass{
				"0": trafficGeneral, "1": trafficReplication, "2": trafficReplication,
			},
			available: []string{"0", "2", "3"},
			targets:   poolTargets{general: 1, replication: 2},
			want: map[string]trafficClass{
				"0": trafficGeneral, "2": trafficReplication, "3": trafficReplication,
			},
		},
		{
			name: "general is refilled before replication",
			previous: map[string]trafficClass{
				"2": trafficReplication, "3": trafficReplication,
			},
			available: []string{"2", "3", "4"},
			targets:   poolTargets{general: 1, replication: 2},
			want: map[string]trafficClass{
				"2": trafficReplication, "3": trafficReplication, "4": trafficGeneral,
			},
		},
		{
			name:      "multiple general sessions",
			available: []string{"d", "b", "a", "c"},
			targets:   poolTargets{general: 2, replication: 2},
			want: map[string]trafficClass{
				"a": trafficGeneral, "b": trafficGeneral, "c": trafficReplication, "d": trafficReplication,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, reconcilePoolAssignments(tt.previous, tt.available, tt.targets))
		})
	}
}

func TestPartitionedClientConnRoutes(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		wantErr     error
		wantGeneral bool
	}{
		{name: "general", method: "/admin", wantErr: errGeneral, wantGeneral: true},
		{name: "replication", method: "/replication", wantErr: errReplication},
		{name: "unknown defaults to general", method: "/unknown", wantErr: errGeneral, wantGeneral: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			general := &recordingPoolClient{invokeErr: errGeneral, streamErr: errGeneral}
			replication := &recordingPoolClient{invokeErr: errReplication, streamErr: errReplication}
			client := newTestPartitionedClientConn(general, replication, nil)

			require.ErrorIs(t, client.Invoke(t.Context(), tt.method, nil, nil), tt.wantErr)
			_, err := client.NewStream(t.Context(), &grpc.StreamDesc{}, tt.method)
			require.ErrorIs(t, err, tt.wantErr)

			if tt.wantGeneral {
				require.Equal(t, []string{tt.method}, general.invoked)
				require.Equal(t, []string{tt.method}, general.streamed)
				require.Empty(t, replication.invoked)
				require.Empty(t, replication.streamed)
			} else {
				require.Empty(t, general.invoked)
				require.Empty(t, general.streamed)
				require.Equal(t, []string{tt.method}, replication.invoked)
				require.Equal(t, []string{tt.method}, replication.streamed)
			}
		})
	}
}

func TestPartitionedClientConnReadiness(t *testing.T) {
	for _, tt := range []struct {
		name        string
		general     bool
		replication bool
		want        bool
	}{
		{name: "both ready", general: true, replication: true, want: true},
		{name: "general unavailable", replication: true},
		{name: "replication unavailable", general: true},
		{name: "both unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestPartitionedClientConn(
				&recordingPoolClient{callable: tt.general},
				&recordingPoolClient{callable: tt.replication},
				nil,
			)
			require.Equal(t, tt.want, client.CanMakeCalls())
		})
	}
}

func TestPartitionedClientConnClose(t *testing.T) {
	for _, tt := range []struct {
		name           string
		generalErr     error
		replicationErr error
	}{
		{name: "no errors"},
		{name: "general error", generalErr: errGeneral},
		{name: "replication error", replicationErr: errReplication},
		{name: "both errors", generalErr: errGeneral, replicationErr: errReplication},
	} {
		t.Run(tt.name, func(t *testing.T) {
			general := &recordingPoolClient{closeErr: tt.generalErr}
			replication := &recordingPoolClient{closeErr: tt.replicationErr}
			client := newTestPartitionedClientConn(general, replication, nil)

			err := client.Close()
			if tt.generalErr != nil {
				require.ErrorIs(t, err, tt.generalErr)
			}
			if tt.replicationErr != nil {
				require.ErrorIs(t, err, tt.replicationErr)
			}
			if tt.generalErr == nil && tt.replicationErr == nil {
				require.NoError(t, err)
			}
			require.Equal(t, err, client.Close())
			require.Equal(t, 1, general.closed)
			require.Equal(t, 1, replication.closed)
		})
	}
}

func TestPartitionedClientConnPublishesSnapshots(t *testing.T) {
	var updates []PoolSnapshot
	general := &recordingPoolClient{description: "general-client"}
	replication := &recordingPoolClient{description: "replication-client"}
	client := newTestPartitionedClientConn(general, replication, func(snapshot PoolSnapshot) {
		updates = append(updates, snapshot)
	})

	client.OnConnectionListUpdate(map[string]session.ManagedMuxSession{
		"2": staticMuxSession{closed: true, info: &session.MuxSessionInfo{State: session.Closed}},
		"0": staticMuxSession{info: &session.MuxSessionInfo{State: session.Connected}},
		"1": staticMuxSession{info: &session.MuxSessionInfo{State: session.Error}},
	})

	require.Equal(t, []string{"0"}, general.stateKeys)
	require.Equal(t, []string{"1", "2"}, replication.stateKeys)
	require.Equal(t, PoolSnapshot{
		General:     PoolStatus{Target: 1, Assigned: 1, Connected: 1},
		Replication: PoolStatus{Target: 2, Assigned: 2},
		Assignments: map[string]string{"0": GeneralPoolName, "1": ReplicationPoolName, "2": ReplicationPoolName},
	}, client.Snapshot())
	require.Equal(t, []PoolSnapshot{client.Snapshot()}, updates)
	require.Contains(t, client.Describe(), "general-client")
	require.Contains(t, client.Describe(), "replication-client")

	// Returned snapshots are immutable copies.
	snapshot := client.Snapshot()
	snapshot.Assignments["0"] = "changed"
	require.Equal(t, GeneralPoolName, client.Snapshot().Assignments["0"])
}

func TestNewPartitionedClientConnValidation(t *testing.T) {
	for _, tt := range []struct {
		name        string
		options     PartitionedClientConnOptions
		wantErrPart string
	}{
		{name: "missing general", options: PartitionedClientConnOptions{ReplicationTarget: 1}, wantErrPart: "at least one"},
		{name: "missing replication", options: PartitionedClientConnOptions{GeneralTarget: 1}, wantErrPart: "at least one"},
		{name: "invalid name", options: PartitionedClientConnOptions{
			Name: "invalid/name", GeneralTarget: 1, ReplicationTarget: 1,
			GeneralDialOptions: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		}, wantErrPart: "create general mux pool"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewPartitionedClientConn(t.Context(), tt.options)
			require.Nil(t, client)
			require.ErrorContains(t, err, tt.wantErrPart)
		})
	}
}

func TestNewPartitionedClientConn(t *testing.T) {
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	client, err := NewPartitionedClientConn(t.Context(), PartitionedClientConnOptions{
		Name:                   "test",
		GeneralTarget:          1,
		ReplicationTarget:      1,
		GeneralDialOptions:     dialOptions,
		ReplicationDialOptions: dialOptions,
	})
	require.NoError(t, err)
	require.NoError(t, client.Close())
}

func TestNewPartitionedClientConnCleansUpPartialConstruction(t *testing.T) {
	general := &recordingPoolClient{}
	calls := 0
	client, err := newPartitionedClientConnWithFactory(t.Context(), PartitionedClientConnOptions{
		Name: "test", GeneralTarget: 1, ReplicationTarget: 1,
	}, func(context.Context, string, ...grpc.DialOption) (managedPoolClient, error) {
		calls++
		if calls == 1 {
			return general, nil
		}
		return nil, errReplication
	})
	require.Nil(t, client)
	require.ErrorIs(t, err, errReplication)
	require.Equal(t, 1, general.closed)
}

func TestPartitionedClientConnContextCancellationClearsSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	updates := make(chan PoolSnapshot, 4)
	clients := []*recordingPoolClient{{}, {}}
	idx := 0
	client, err := newPartitionedClientConnWithFactory(ctx, PartitionedClientConnOptions{
		Name: "test", GeneralTarget: 1, ReplicationTarget: 1,
		IsReplication: isTestReplication,
		OnUpdate:      func(snapshot PoolSnapshot) { updates <- snapshot },
	}, func(context.Context, string, ...grpc.DialOption) (managedPoolClient, error) {
		client := clients[idx]
		idx++
		return client, nil
	})
	require.NoError(t, err)
	initial := <-updates
	require.Zero(t, initial.General.Assigned)
	require.Zero(t, initial.Replication.Assigned)
	client.OnConnectionListUpdate(map[string]session.ManagedMuxSession{
		"0": staticMuxSession{info: &session.MuxSessionInfo{State: session.Connected}},
		"1": staticMuxSession{info: &session.MuxSessionInfo{State: session.Connected}},
	})
	assigned := <-updates
	require.Equal(t, 1, assigned.General.Assigned)
	require.Equal(t, 1, assigned.Replication.Assigned)
	cancel()

	var cleared PoolSnapshot
	select {
	case cleared = <-updates:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cleared pool snapshot")
	}
	require.Zero(t, cleared.General.Assigned)
	require.Zero(t, cleared.Replication.Assigned)
	require.Zero(t, client.Snapshot().General.Assigned)
	require.Zero(t, client.Snapshot().Replication.Assigned)
}

func newTestPartitionedClientConn(
	general *recordingPoolClient,
	replication *recordingPoolClient,
	onUpdate func(PoolSnapshot),
) *PartitionedClientConn {
	return newPartitionedClientConn(
		general,
		replication,
		poolTargets{general: 1, replication: 2},
		isTestReplication,
		onUpdate,
	)
}

func isTestReplication(method string) bool { return method == "/replication" }

var (
	errGeneral     = errors.New("general")
	errReplication = errors.New("replication")
)

type recordingPoolClient struct {
	invoked     []string
	streamed    []string
	stateKeys   []string
	invokeErr   error
	streamErr   error
	closeErr    error
	callable    bool
	closed      int
	description string
}

func (c *recordingPoolClient) Invoke(_ context.Context, method string, _ any, _ any, _ ...grpc.CallOption) error {
	c.invoked = append(c.invoked, method)
	return c.invokeErr
}

func (c *recordingPoolClient) NewStream(_ context.Context, _ *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	c.streamed = append(c.streamed, method)
	return noopClientStream{}, c.streamErr
}

func (c *recordingPoolClient) Close() error {
	c.closed++
	return c.closeErr
}

func (c *recordingPoolClient) Describe() string   { return c.description }
func (c *recordingPoolClient) CanMakeCalls() bool { return c.callable }
func (c *recordingPoolClient) UpdateState(conns map[string]func() (net.Conn, error)) {
	c.stateKeys = c.stateKeys[:0]
	for id := range conns {
		c.stateKeys = append(c.stateKeys, id)
	}
	slices.Sort(c.stateKeys)
}

type noopClientStream struct{}

func (noopClientStream) Header() (metadata.MD, error) { return nil, nil }
func (noopClientStream) Trailer() metadata.MD         { return nil }
func (noopClientStream) CloseSend() error             { return nil }
func (noopClientStream) Context() context.Context     { return context.Background() }
func (noopClientStream) SendMsg(any) error            { return nil }
func (noopClientStream) RecvMsg(any) error            { return nil }

type staticMuxSession struct {
	closed bool
	info   *session.MuxSessionInfo
}

func (s staticMuxSession) IsClosed() bool                        { return s.closed }
func (staticMuxSession) Close()                                  {}
func (staticMuxSession) CloseChan() <-chan struct{}              { return nil }
func (staticMuxSession) Open() (net.Conn, error)                 { return nil, nil }
func (s staticMuxSession) State() *session.MuxSessionInfo        { return s.info }
func (staticMuxSession) Describe() string                        { return "static" }
func (staticMuxSession) GetConnectionInfo() (net.Addr, net.Addr) { return nil, nil }
