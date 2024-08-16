package proxy

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
)

const (
	testLocalServerAddress    = "localhost:7266"
	testRemoteServerAddress   = "localhost:8266"
	testInboundServerAddress  = "localhost:5366"
	testOutboundServerAddress = "localhost:5466"
)

type (
	proxyTestSuite struct {
		suite.Suite
		ctrl *gomock.Controller
	}

	mockConfig struct {
		localServerAddress    string
		remoteServerAddress   string
		inboundServerAddress  string
		outboundServerAddress string
	}
)

func (mc *mockConfig) GetGRPCServerOptions() []grpc.ServerOption {
	return nil
}

func (mc *mockConfig) GetOutboundServerAddress() string {
	return mc.outboundServerAddress
}

func (mc *mockConfig) GetInboundServerAddress() string {
	return mc.inboundServerAddress
}

func (mc *mockConfig) GetRemoteServerRPCAddress() string {
	return mc.remoteServerAddress
}

func (mc *mockConfig) GetLocalServerRPCAddress() string {
	return mc.localServerAddress
}

func TestAdminHandlerSuite(t *testing.T) {
	s := new(proxyTestSuite)
	suite.Run(t, s)
}

func (s *proxyTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())

}

func (s *proxyTestSuite) TearDownTest() {
}

func (s *proxyTestSuite) Test_StartStop() {
	config := &mockConfig{
		outboundServerAddress: testOutboundServerAddress,
		inboundServerAddress:  testInboundServerAddress,
		remoteServerAddress:   testRemoteServerAddress,
		localServerAddress:    testLocalServerAddress,
	}

	logger := log.NewTestLogger()
	rpcFactory := rpc.NewRPCFactory(config, logger)
	clientFactory := client.NewClientFactory(rpcFactory, logger)
	proxy := NewProxy(
		config,
		logger,
		clientFactory,
	)

	proxy.Start()
	proxy.Stop()
}
