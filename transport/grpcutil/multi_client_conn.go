package grpcutil

import (
	"context"
	"fmt"
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
	connMapLock sync.RWMutex
	connMap     map[string]func() (net.Conn, error)
	resolver    manual.Resolver
	clientConn  *grpc.ClientConn
}

type ConnProvider func() (map[string]func() (net.Conn, error), error)

func NewMultiClientConn(name string, connProvider ConnProvider, opts ...grpc.DialOption) (*MultiClientConn, error) {
	mcc := &MultiClientConn{}
	var err error
	dialOpts := make([]grpc.DialOption, len(opts)+2)
	manualResolver := manual.NewBuilderWithScheme(scheme)
	manualResolver.BuildCallback = func(resolver.Target, resolver.ClientConn, resolver.BuildOptions) {
		mcc.connMapLock.Lock()
		defer mcc.connMapLock.Unlock()
		mcc.connMap, err = connProvider()
		manualResolver.InitialState(mcc.stateFromConns())
		if err != nil {
			panic(err)
		}
	}
	dialOpts[0] = grpc.WithResolvers(manualResolver)
	dialOpts[1] = grpc.WithContextDialer(mcc.getMapDialer())
	copy(dialOpts[2:], opts)
	mcc.clientConn, err = grpc.NewClient(fmt.Sprintf("%s://%s", scheme, name),
		dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create underlying grpc client")
	}
	return mcc, nil
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

func (mcc *MultiClientConn) stateFromConns() resolver.State {
	newState := resolver.State{
		Addresses: make([]resolver.Address, len(mcc.connMap)),
	}
	idx := 0
	for addr := range mcc.connMap {
		newState.Addresses[idx] = resolver.Address{Addr: addr}
		idx++
	}
	return newState
}

func (mcc *MultiClientConn) UpdateConnections(conns map[string]func() (net.Conn, error)) error {
	mcc.connMapLock.Lock()
	defer mcc.connMapLock.Unlock()
	mcc.connMap = conns
	mcc.resolver.UpdateState(mcc.stateFromConns())
	return nil
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
