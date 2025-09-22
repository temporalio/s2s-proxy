package grpcutil

import (
	"context"
	"fmt"
	"maps"
	"net"
	"sync"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

const (
	scheme = "multiclient"
)

// MultiClientConn is a wrapper over grpc.ClientConn that allows it to be configured over a set of existing
// connections. Each existing connection is represented as a single "Endpoint", which will load balance as expected
// when configuring the MultiClientConn with load balancing policies like {"loadBalancingPolicy":"round_robin"}
// Do not use the grpc.DialOption grpc.WithResolvers or grpc.WithContextDialer with MultiClientConn! We cannot detect
// when they are used, and they will break MultiClientConn.
// Note: This is not ready for use in production yet, ClientConn.Connect() and ClientConn.UpdateState() cannot yet be called properly.
type MultiClientConn struct {
	// connMapLock is being used with connMap over a sync.Map for now. If using a MultiClientConn on large numbers of
	// muxes, it's probably best to switch to sync.Map for the sharded read locks
	connMapLock sync.RWMutex
	// connMap contains the mapping of addresses to connections. We avoid storing the net.Conn directly to ensure
	// yamux always sees new calls to Open and can open new streams properly
	connMap map[string]func() (net.Conn, error)
	// resolver generates map keys that will match to the Dialer's connection map
	resolver *manual.Resolver
	// clientConn handles the calls to our resolver and Dialer
	clientConn *grpc.ClientConn
}

func NewMultiClientConn(name string, opts ...grpc.DialOption) (*MultiClientConn, error) {
	mcc := &MultiClientConn{}
	var err error
	dialOpts := make([]grpc.DialOption, len(opts)+2)
	mcc.resolver = manual.NewBuilderWithScheme(scheme)
	dialOpts[0] = grpc.WithResolvers(mcc.resolver)
	dialOpts[1] = grpc.WithContextDialer(mcc.getMapDialer())
	copy(dialOpts[2:], opts)
	mcc.clientConn, err = grpc.NewClient(fmt.Sprintf("%s://%s", scheme, name),
		dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create underlying grpc client")
	}
	// UpdateState will panic if this isn't called first, or a connection hasn't been attempted yet.
	mcc.clientConn.Connect()
	return mcc, nil
}

func (mcc *MultiClientConn) UpdateState(conns map[string]func() (net.Conn, error)) {
	mcc.connMapLock.Lock()
	defer mcc.connMapLock.Unlock()
	// Make sure we don't hold onto a mutable pointer to the original map
	mcc.connMap = maps.Clone(conns)
	mcc.resolver.UpdateState(mcc.deriveStateFromConns())
}

// grpc.ClientConnInterface

// Invoke forwards to grpc.ClientConn
func (mcc *MultiClientConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	if mcc.clientConn == nil {
		return errors.New("invoke called with uninitialized MultiClientConn")
	}
	return mcc.clientConn.Invoke(ctx, method, args, reply, opts...)
}

// NewStream forwards to grpc.ClientConn
func (mcc *MultiClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if mcc.clientConn == nil {
		return nil, errors.New("newStream called with uninitialized MultiClientConn")
	}
	return mcc.clientConn.NewStream(ctx, desc, method, opts...)
}

func (mcc *MultiClientConn) getMapDialer() func(ctx context.Context, addr string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		mcc.connMapLock.RLock()
		connFn, exists := mcc.connMap[addr]
		mcc.connMapLock.RUnlock()
		if exists {
			return connFn()
		}
		return nil, fmt.Errorf("connection key %s didn't match a connection", addr)
	}
}

// deriveStateFromConns creates one endpoint for each registered connection in the connMap. Each endpoint only has
// a single address.
func (mcc *MultiClientConn) deriveStateFromConns() resolver.State {
	newState := resolver.State{
		Endpoints: make([]resolver.Endpoint, len(mcc.connMap)),
	}
	idx := 0
	for addr := range mcc.connMap {
		newState.Endpoints[idx] = resolver.Endpoint{Addresses: []resolver.Address{{Addr: addr}}}
		idx++
	}
	return newState
}
