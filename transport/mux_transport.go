package transport

import (
	"context"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type (
	muxTransport struct {
		config  config.MuxTransportConfig
		session *yamux.Session
		isReady bool
	}
)

func (s *muxTransport) Connect() (*grpc.ClientConn, error) {
	conn, err := s.session.Open()
	if err != nil {
		return nil, err
	}

	// Create a gRPC dialer using the existing connection
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return conn, nil // Return the original TCP connection
	}

	// Establish a gRPC client connection using the custom dialer
	return grpc.Dial(
		"unused", // Address is ignored since we're using a custom dialer
		grpc.WithTransportCredentials(insecure.NewCredentials()), // No TLS for simplicity
		grpc.WithContextDialer(dialer),
	)
}

func (s *muxTransport) Serve(server *grpc.Server) error {
	return server.Serve(s.session)
}
