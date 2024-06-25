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
	"sync"

	"google.golang.org/grpc"

	"github.com/temporalio/temporal-proxy/config"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/primitives"
	"go.temporal.io/server/common/rpc/encryption"
)

type (

	// RPCFactory creates gRPC listener and connection.
	RPCFactory interface {
		CreateRemoteFrontendGRPCConnection(rpcAddress string) *grpc.ClientConn
	}

	// rpcFactory is an implementation of common.rpcFactory interface
	rpcFactory struct {
		config      *config.RPC
		serviceName primitives.ServiceName
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
func NewFactory(
	cfg *config.RPC,
	sName primitives.ServiceName,
	logger log.Logger,
	tlsProvider encryption.TLSConfigProvider,
	frontendURL string,
	frontendTLSConfig *tls.Config,
) *rpcFactory {
	return &rpcFactory{
		config:            cfg,
		serviceName:       sName,
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

func (d *rpcFactory) dial(hostName string, tlsClientConfig *tls.Config) *grpc.ClientConn {
	connection, err := Dial(hostName, tlsClientConfig, d.logger)
	if err != nil {
		d.logger.Fatal("Failed to create gRPC connection", tag.Error(err))
		return nil
	}

	return connection
}
