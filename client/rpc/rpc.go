// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
	"go.temporal.io/server/common/rpc/encryption"
	"google.golang.org/grpc"
)

const (
	// LocalhostIP default localhost
	LocalhostIP = "LOCALHOST_IP"
	// Localhost default hostname
	LocalhostIPDefault = "127.0.0.1"
)

type (
	GrpcServerOptions struct {
		Options           []grpc.ServerOption
		UnaryInterceptors []grpc.UnaryServerInterceptor
	}

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

		frontendURL       string
		frontendTLSConfig *tls.Config

		initListener sync.Once
		grpcListener net.Listener
		tlsFactory   TLSConfigProvider
	}
)

// NewFactory builds a new RPCFactory
// conforming to the underlying configuration
func NewRPCFactory(
	config config.Config,
	logger log.Logger,
	tlsProvider encryption.TLSConfigProvider,
	frontendURL string,
	frontendTLSConfig *tls.Config,
) RPCFactory {
	return &rpcFactory{
		serviceName:       "proxy",
		logger:            logger,
		frontendURL:       frontendURL,
		frontendTLSConfig: frontendTLSConfig,
		tlsFactory:        tlsProvider,
	}
}

func (d *rpcFactory) GetRemoteClusterClientConfig(hostname string) (*tls.Config, error) {
	if d.tlsFactory != nil {
		return d.tlsFactory.GetRemoteClusterClientConfig(hostname)
	}

	return nil, nil
}

// CreateRemoteFrontendGRPCConnection creates connection for gRPC calls
func (d *rpcFactory) CreateRemoteFrontendGRPCConnection(rpcAddress string) *grpc.ClientConn {
	var tlsClientConfig *tls.Config
	var err error
	if d.tlsFactory != nil {
		hostname, _, err2 := net.SplitHostPort(rpcAddress)
		if err2 != nil {
			d.logger.Fatal("Invalid rpcAddress for remote cluster", tag.Error(err2))
		}
		tlsClientConfig, err = d.tlsFactory.GetRemoteClusterClientConfig(hostname)

		if err != nil {
			d.logger.Fatal("Failed to create tls config for gRPC connection", tag.Error(err))
			return nil
		}
	}

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
