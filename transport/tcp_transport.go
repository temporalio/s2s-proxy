package transport

import (
	"crypto/tls"
	"net"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

type (
	tcpClient struct {
		config config.TCPClientSetting
	}

	tcpServer struct {
		config config.TCPServerSetting
	}
)

func (c *tcpClient) Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error) {
	var tlsConfig *tls.Config
	var err error
	if tls := c.config.TLS; tls.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(tls)
		if err != nil {
			return nil, err
		}
	}

	return grpcutil.Dial(c.config.ServerAddress, tlsConfig, clientMetrics, nil)
}

func (s *tcpServer) Serve(server *grpc.Server) error {
	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return err
	}

	return server.Serve(listener)
}

func (s *tcpServer) IsClosed() bool {
	return false
}
