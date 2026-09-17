package extension

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

// Connections maps an extension server name to a connection to that server.
//
// The value type is deliberately the narrow [grpc.ClientConnInterface] rather
// than *grpc.ClientConn: it has no Close, so a key opened over one of these
// cannot tear down a connection it shares with every other key on the same
// server. Closing is [Dial]'s business, not its callers'.
type Connections map[string]grpc.ClientConnInterface

// Dial opens a connection to every configured extension server, keyed by name,
// and ties each one's lifetime to lifetime: when that context is done, the
// connections are closed.
//
// Connecting is lazy, as gRPC always is, so a server that is merely unreachable
// is not an error here - it surfaces on the first wrap or unwrap. What is
// checked now is what can be: a name collision, unusable TLS material, and a
// target gRPC will not accept. A failure closes whatever already opened rather
// than relying on the caller to cancel lifetime.
//
// This is deliberately narrower than the proxy's upstream dialing. An extension
// server is not a Temporal service, so it gets neither namespace translation nor
// the proxy's codec, and it is never sealed with the vault it backs - doing so
// would ask it to unwrap the key protecting its own traffic.
func Dial(lifetime context.Context, servers []config.ExtensionServer) (Connections, error) {
	conns := make(Connections, len(servers))
	opened := make([]*grpc.ClientConn, 0, len(servers))

	closeOpened := func() {
		for _, cc := range opened {
			_ = cc.Close()
		}
	}

	for _, s := range servers {
		// Config rejects duplicates, but Dial may be handed a config nobody
		// validated, and a duplicate would silently drop a server rather than
		// fail.
		if _, dup := conns[s.Name]; dup {
			closeOpened()
			return nil, fmt.Errorf("duplicate extension server name: %s", s.Name)
		}

		cc, err := dialOne(s)
		if err != nil {
			closeOpened()
			return nil, err
		}

		context.AfterFunc(lifetime, func() { _ = cc.Close() })

		conns[s.Name] = cc
		opened = append(opened, cc)
	}

	return conns, nil
}

// dialOne builds the connection for a single extension server. A TLS block that
// is not enabled yields a nil config, which is what asks for an insecure
// connection.
func dialOne(s config.ExtensionServer) (*grpc.ClientConn, error) {
	tlsConfig, err := encryption.GetClientTLSConfig(s.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid TLS config for extension server %q: %w", s.Name, err)
	}

	cc, err := grpc.NewClient(
		s.Address,
		grpcutil.MakeExtensionDialOptions(tlsConfig, metrics.GetGRPCClientMetrics("extension"))...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial extension server %q at %s: %w", s.Name, s.Address, err)
	}

	return cc, nil
}
