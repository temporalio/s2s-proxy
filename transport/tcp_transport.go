package transport

import (
	"crypto/tls"
	"net"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"google.golang.org/grpc"
)

type (
	tcpClient struct {
		config config.TCPClientSetting
	}

	tcpServer struct {
		config config.TCPServerSetting
	}
)

func (c *tcpClient) Connect() (*grpc.ClientConn, error) {
	var tlsConfig *tls.Config
	var err error
	if tls := c.config.TLS; tls.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(tls)
		if err != nil {
			return nil, err
		}
	}

	return dial(c.config.ServerAddress, tlsConfig, nil)
}

func (s *tcpServer) Serve(server *grpc.Server) error {
	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return err
	}

	return server.Serve(listener)
}
