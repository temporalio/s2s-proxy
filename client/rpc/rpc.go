package rpc

import (
	"crypto/tls"
	"net"
	"os"
	"sync"

	"github.com/temporalio/temporal-proxy/common"
	"github.com/temporalio/temporal-proxy/config"

	"go.temporal.io/server/common/convert"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

const (
	// LocalhostIP default localhost
	LocalhostIP = "LOCALHOST_IP"
	// Localhost default hostname
	LocalhostIPDefault = "127.0.0.1"
)

type (
	// RPCFactory creates gRPC listener and connection.
	RPCFactory interface {
		CreateRemoteFrontendGRPCConnection(rpcAddress string) *grpc.ClientConn
		GetGRPCListener() net.Listener
	}

	// rpcFactory is an implementation of common.rpcFactory interface
	rpcFactory struct {
		config      config.Config
		serviceName string
		logger      log.Logger

		initListener sync.Once
		grpcListener net.Listener
	}
)

// NewFactory builds a new RPCFactory
// conforming to the underlying configuration
func NewRPCFactory(
	config config.Config,
	logger log.Logger,
) RPCFactory {
	return &rpcFactory{
		serviceName: "proxy",
		config:      config,
		logger:      logger,
	}
}

// CreateRemoteFrontendGRPCConnection creates connection for gRPC calls
// TODO: TLS config
func (d *rpcFactory) CreateRemoteFrontendGRPCConnection(rpcAddress string) *grpc.ClientConn {
	var tlsClientConfig *tls.Config
	return d.dial(rpcAddress, tlsClientConfig)
}

// GetGRPCListener returns cached dispatcher for gRPC inbound or creates one
func (d *rpcFactory) GetGRPCListener() net.Listener {
	d.initListener.Do(func() {
		hostAddress := net.JoinHostPort(getLocalhostIP(), convert.IntToString(d.config.GetGRPCPort()))
		var err error
		d.grpcListener, err = net.Listen("tcp", hostAddress)

		if err != nil {
			d.logger.Fatal("Failed to start gRPC listener", tag.Error(err), common.ServiceTag(d.serviceName), tag.Address(hostAddress))
		}

		d.logger.Info("Created gRPC listener", common.ServiceTag(d.serviceName), tag.Address(hostAddress))
	})

	return d.grpcListener
}

func (d *rpcFactory) dial(hostName string, tlsClientConfig *tls.Config) *grpc.ClientConn {
	connection, err := Dial(hostName, tlsClientConfig, d.logger)
	if err != nil {
		d.logger.Fatal("Failed to create gRPC connection", tag.Error(err))
		return nil
	}

	return connection
}

func lookupLocalhostIP(domain string) string {
	// lookup localhost and favor the first ipv4 address
	// unless there are only ipv6 addresses available
	ips, err := net.LookupIP(domain)
	if err != nil || len(ips) == 0 {
		// fallback to default instead of error
		return LocalhostIPDefault
	}
	var listenIp net.IP
	for _, ip := range ips {
		listenIp = ip
		if listenIp.To4() != nil {
			break
		}
	}
	return listenIp.String()
}

// GetLocalhostIP returns the ip address of the localhost domain
func getLocalhostIP() string {
	localhostIP := os.Getenv(LocalhostIP)
	ip := net.ParseIP(localhostIP)
	if ip != nil {
		// if localhost is an ip return it
		return ip.String()
	}
	// otherwise, ignore the value and lookup `localhost`
	return lookupLocalhostIP("localhost")
}
