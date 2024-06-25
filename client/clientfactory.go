// The MIT License

// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.

// Copyright (c) 2020 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package client

import (
	"time"

	"github.com/temporalio/temporal-proxy/client/admin"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
)

type (
	// Factory can be used to create RPC clients for temporal services
	Factory interface {
		NewRemoteAdminClientWithTimeout(rpcAddress string, timeout time.Duration, largeTimeout time.Duration) adminservice.AdminServiceClient
	}

	rpcClientFactory struct {
		rpcFactory      RPCFactory
		metricsHandler  metrics.Handler
		logger          log.Logger
		throttledLogger log.Logger
	}
)

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	rpcFactory common.RPCFactory,
	logger log.Logger,
	throttledLogger log.Logger,
) Factory {
	return &rpcClientFactory{
		rpcFactory:      rpcFactory,
		logger:          logger,
		throttledLogger: throttledLogger,
	}
}

func (cf *rpcClientFactory) NewRemoteAdminClientWithTimeout(
	rpcAddress string,
	timeout time.Duration,
	largeTimeout time.Duration,
) adminservice.AdminServiceClient {
	connection := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(rpcAddress)
	client := adminservice.NewAdminServiceClient(connection)
	return cf.newAdminClient(client, timeout, largeTimeout)
}

func (cf *rpcClientFactory) newAdminClient(
	client adminservice.AdminServiceClient,
	timeout time.Duration,
	longPollTimeout time.Duration,
) adminservice.AdminServiceClient {
	client = admin.NewClient(timeout, longPollTimeout, client)
	if cf.metricsHandler != nil {
		client = admin.NewMetricClient(client, cf.metricsHandler, cf.throttledLogger)
	}
	return client
}
