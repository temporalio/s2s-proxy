package grpcutil

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"sync"

	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

const (
	// GeneralPoolName identifies the pool used for non-replication RPCs.
	GeneralPoolName = "general"
	// ReplicationPoolName identifies the pool used for replication RPCs.
	ReplicationPoolName = "replication"
)

// PoolStatus describes the configured and current size of one mux pool.
type PoolStatus struct {
	Target    int
	Assigned  int
	Connected int
}

// PoolSnapshot is a detached point-in-time view of both mux pools.
type PoolSnapshot struct {
	General     PoolStatus
	Replication PoolStatus
	Assignments map[string]string
}

// PartitionedClientConnOptions configures two disjoint mux connection pools.
type PartitionedClientConnOptions struct {
	Name                   string
	GeneralTarget          int
	ReplicationTarget      int
	IsReplication          func(fullMethodName string) bool
	GeneralDialOptions     []grpc.DialOption
	ReplicationDialOptions []grpc.DialOption
	OnUpdate               func(PoolSnapshot)
}

type trafficClass uint8

const (
	trafficGeneral trafficClass = iota
	trafficReplication
)

func (c trafficClass) String() string {
	if c == trafficReplication {
		return ReplicationPoolName
	}
	return GeneralPoolName
}

type poolTargets struct {
	general     int
	replication int
}

type managedPoolClient interface {
	grpc.ClientConnInterface
	Close() error
	Describe() string
	CanMakeCalls() bool
	UpdateState(map[string]func() (net.Conn, error))
}

type poolClientFactory func(context.Context, string, ...grpc.DialOption) (managedPoolClient, error)

// PartitionedClientConn presents one ClientConnInterface while keeping the
// underlying physical mux sessions in disjoint general and replication pools.
// It is also the mux manager's connection listener.
type PartitionedClientConn struct {
	general       managedPoolClient
	replication   managedPoolClient
	targets       poolTargets
	isReplication func(string) bool
	onUpdate      func(PoolSnapshot)
	lifetime      context.Context

	updateMu sync.Mutex
	mu       sync.RWMutex

	assignments map[string]trafficClass
	sessions    map[string]session.ManagedMuxSession
	closeOnce   sync.Once
	closeErr    error
}

// NewPartitionedClientConn creates a client that routes RPCs across two disjoint mux pools.
func NewPartitionedClientConn(ctx context.Context, opts PartitionedClientConnOptions) (*PartitionedClientConn, error) {
	return newPartitionedClientConnWithFactory(ctx, opts, func(ctx context.Context, name string, dialOptions ...grpc.DialOption) (managedPoolClient, error) {
		return NewMultiClientConn(ctx, name, dialOptions...)
	})
}

func newPartitionedClientConnWithFactory(
	ctx context.Context,
	opts PartitionedClientConnOptions,
	factory poolClientFactory,
) (*PartitionedClientConn, error) {
	if opts.GeneralTarget < 1 || opts.ReplicationTarget < 1 {
		return nil, fmt.Errorf("mux pool targets must both be at least one: general=%d replication=%d",
			opts.GeneralTarget, opts.ReplicationTarget)
	}

	general, err := factory(ctx, opts.Name+"-general", opts.GeneralDialOptions...)
	if err != nil {
		return nil, fmt.Errorf("create general mux pool: %w", err)
	}
	replication, err := factory(ctx, opts.Name+"-replication", opts.ReplicationDialOptions...)
	if err != nil {
		_ = general.Close()
		return nil, fmt.Errorf("create replication mux pool: %w", err)
	}

	client := newPartitionedClientConn(
		general,
		replication,
		poolTargets{general: opts.GeneralTarget, replication: opts.ReplicationTarget},
		opts.IsReplication,
		opts.OnUpdate,
	)
	client.lifetime = ctx
	client.notify(client.Snapshot())
	context.AfterFunc(ctx, client.clearState)
	return client, nil
}

func newPartitionedClientConn(
	general managedPoolClient,
	replication managedPoolClient,
	targets poolTargets,
	isReplication func(string) bool,
	onUpdate func(PoolSnapshot),
) *PartitionedClientConn {
	return &PartitionedClientConn{
		general:       general,
		replication:   replication,
		targets:       targets,
		isReplication: isReplication,
		onUpdate:      onUpdate,
		assignments:   make(map[string]trafficClass),
		sessions:      make(map[string]session.ManagedMuxSession),
	}
}

// OnConnectionListUpdate satisfies mux.ConnListener. The mux manager invokes
// it with a complete snapshot whenever a session appears or disappears.
func (c *PartitionedClientConn) OnConnectionListUpdate(sessions map[string]session.ManagedMuxSession) {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()

	if c.lifetime != nil && c.lifetime.Err() != nil {
		sessions = nil
	}

	c.mu.RLock()
	previous := maps.Clone(c.assignments)
	c.mu.RUnlock()

	available := make([]string, 0, len(sessions))
	for id := range sessions {
		available = append(available, id)
	}
	assignments := reconcilePoolAssignments(previous, available, c.targets)
	general := make(map[string]func() (net.Conn, error), c.targets.general)
	replication := make(map[string]func() (net.Conn, error), c.targets.replication)
	for id, managedSession := range sessions {
		switch assignments[id] {
		case trafficGeneral:
			general[id] = managedSession.Open
		case trafficReplication:
			replication[id] = managedSession.Open
		}
	}

	// Resolver updates can execute external code, so do not hold the state lock.
	c.general.UpdateState(general)
	c.replication.UpdateState(replication)

	c.mu.Lock()
	c.assignments = assignments
	c.sessions = maps.Clone(sessions)
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.notify(snapshot)
}

func reconcilePoolAssignments(
	previous map[string]trafficClass,
	available []string,
	targets poolTargets,
) map[string]trafficClass {
	slices.Sort(available)
	present := make(map[string]struct{}, len(available))
	for _, id := range available {
		present[id] = struct{}{}
	}

	assignments := make(map[string]trafficClass, len(available))
	generalCount := 0
	for id, class := range previous {
		if _, ok := present[id]; !ok {
			continue
		}
		if class != trafficGeneral && class != trafficReplication {
			continue
		}
		assignments[id] = class
		if class == trafficGeneral {
			generalCount++
		}
	}

	for _, id := range available {
		if _, assigned := assignments[id]; assigned {
			continue
		}
		class := trafficReplication
		if generalCount < targets.general {
			class = trafficGeneral
			generalCount++
		}
		assignments[id] = class
	}
	return assignments
}

func (c *PartitionedClientConn) clientForMethod(method string) managedPoolClient {
	if c.isReplication != nil && c.isReplication(method) {
		return c.replication
	}
	return c.general
}

func (c *PartitionedClientConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	return c.clientForMethod(method).Invoke(ctx, method, args, reply, opts...)
}

func (c *PartitionedClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return c.clientForMethod(method).NewStream(ctx, desc, method, opts...)
}

func (c *PartitionedClientConn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = errors.Join(c.general.Close(), c.replication.Close())
		c.clearState()
	})
	return c.closeErr
}

func (c *PartitionedClientConn) CanMakeCalls() bool {
	return c.general.CanMakeCalls() && c.replication.CanMakeCalls()
}

func (c *PartitionedClientConn) Describe() string {
	snapshot := c.Snapshot()
	return fmt.Sprintf("[PartitionedClientConn general=%s status=%+v, replication=%s status=%+v]",
		c.general.Describe(), snapshot.General, c.replication.Describe(), snapshot.Replication)
}

func (c *PartitionedClientConn) Snapshot() PoolSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshotLocked()
}

func (c *PartitionedClientConn) snapshotLocked() PoolSnapshot {
	snapshot := PoolSnapshot{
		General:     PoolStatus{Target: c.targets.general},
		Replication: PoolStatus{Target: c.targets.replication},
		Assignments: make(map[string]string, len(c.assignments)),
	}
	for id, class := range c.assignments {
		status := &snapshot.General
		if class == trafficReplication {
			status = &snapshot.Replication
		}
		status.Assigned++
		managedSession := c.sessions[id]
		if managedSession != nil && !managedSession.IsClosed() {
			state := managedSession.State()
			if state != nil && state.State == session.Connected {
				status.Connected++
			}
		}
		snapshot.Assignments[id] = class.String()
	}
	return snapshot
}

func (c *PartitionedClientConn) clearState() {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	c.mu.Lock()
	c.assignments = make(map[string]trafficClass)
	c.sessions = make(map[string]session.ManagedMuxSession)
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.notify(snapshot)
}

func (c *PartitionedClientConn) notify(snapshot PoolSnapshot) {
	if c.onUpdate != nil {
		c.onUpdate(snapshot)
	}
}
