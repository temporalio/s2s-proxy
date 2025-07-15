package proxy

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	replication "go.temporal.io/server/api/replication/v1"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/adminservicemock/v1"
	"go.temporal.io/server/common/log"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	clientmock "github.com/temporalio/s2s-proxy/mocks/client"
)

func TestAdminserviceSuite(t *testing.T) {
	suite.Run(t, new(adminserviceSuite))
}

type adminserviceSuite struct {
	suite.Suite
	ctrl *gomock.Controller

	adminClientMock   *adminservicemock.MockAdminServiceClient
	clientFactoryMock *clientmock.MockClientFactory
}

func (s *adminserviceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.adminClientMock = adminservicemock.NewMockAdminServiceClient(s.ctrl)
	s.clientFactoryMock = clientmock.NewMockClientFactory(s.ctrl)
}

func (s *adminserviceSuite) AfterTest() {
	s.ctrl.Finish()
}

func (s *adminserviceSuite) newAdminServiceProxyServer(opts proxyOptions) adminservice.AdminServiceServer {
	cfg := config.ProxyClientConfig{
		TCPClientSetting: config.TCPClientSetting{
			ServerAddress: "fake-forward-address",
			TLS:           encryption.ClientTLSConfig{},
		},
	}
	s.clientFactoryMock.EXPECT().NewRemoteAdminClient(cfg).Return(s.adminClientMock, nil).Times(1)
	return NewAdminServiceProxyServer("test-service-name", cfg, s.clientFactoryMock, opts, log.NewTestLogger())
}

func (s *adminserviceSuite) TestRequestLimit() {
	testReq := &adminservice.StreamWorkflowReplicationMessagesRequest{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
			SyncReplicationState: &replication.SyncReplicationState{
				InclusiveLowWatermark: 100,
				HighPriorityState: &replication.ReplicationState{
					InclusiveLowWatermark: 50,
				},
				LowPriorityState: &replication.ReplicationState{
					InclusiveLowWatermark: 25,
				},
			},
		},
	}

	testResp := &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replication.WorkflowReplicationMessages{
				ExclusiveHighWatermark: 150,
				ReplicationTasks: []*replication.ReplicationTask{
					{
						SourceTaskId: 1,
						TaskType:     1,
					},
				},
			},
		},
	}
	cases := []struct {
		name                string
		messageCount        int
		messagesBeforeClose int
	}{
		{
			name:                "Normal EOF session",
			messageCount:        100,
			messagesBeforeClose: 200,
		},
		{
			name:                "Request limit enforcement",
			messageCount:        100,
			messagesBeforeClose: 50,
		},
		{
			name:                "Request limit matches EOF",
			messageCount:        100,
			messagesBeforeClose: 100,
		},
	}
	for _, tc := range cases {
		s.SetupTest()
		s.Run(tc.name, func() {
			initiatingServerStream := adminservicemock.NewMockAdminService_StreamWorkflowReplicationMessagesServer(s.ctrl)
			destinationServerStream := adminservicemock.NewMockAdminService_StreamWorkflowReplicationMessagesClient(s.ctrl)

			minTimes := min(tc.messageCount, tc.messagesBeforeClose)
			maxTimes := max(tc.messageCount, tc.messagesBeforeClose) + 2 // One more recv than usually expected, because we proceed to block on the last recv

			// Messages from initiator to destination. Counts are atomic because two goroutines are incrementing these
			initiatingServerRecvCount, destinationServerSentCount := atomic.Uint32{}, atomic.Uint32{}
			initiatingServerStream.EXPECT().Recv().DoAndReturn(func() (*adminservice.StreamWorkflowReplicationMessagesRequest, error) {
				initiatingServerRecvCount.Add(1)
				return testReq, nil
			}).MinTimes(minTimes).MaxTimes(maxTimes)
			initiatingServerStream.EXPECT().Recv().DoAndReturn(func() (*adminservice.StreamWorkflowReplicationMessagesRequest, error) {
				initiatingServerRecvCount.Add(1)
				time.Sleep(1 * time.Second)
				return testReq, nil
			}).AnyTimes()
			initiatingServerStream.EXPECT().Recv().Return(nil, io.EOF).AnyTimes()
			destinationServerStream.EXPECT().Send(gomock.Any()).Do(func(_ *adminservice.StreamWorkflowReplicationMessagesRequest) error {
				destinationServerSentCount.Add(1)
				return nil
			}).MinTimes(minTimes).MaxTimes(maxTimes)
			destinationServerStream.EXPECT().Send(gomock.Any()).Return(io.EOF).AnyTimes()
			destinationServerStream.EXPECT().CloseSend().Return(nil).AnyTimes()

			// Messages from destination to initiator. These are AnyTimes because the responses will be cancelled randomly
			destinationServerRecvCount, initiatingServerSentCount := atomic.Uint32{}, atomic.Uint32{}
			destinationServerStream.EXPECT().Recv().DoAndReturn(func() (*adminservice.StreamWorkflowReplicationMessagesResponse, error) {
				destinationServerRecvCount.Add(1)
				return testResp, nil
			}).AnyTimes()
			initiatingServerStream.EXPECT().Send(gomock.Any()).Do(func(_ *adminservice.StreamWorkflowReplicationMessagesResponse) error {
				initiatingServerSentCount.Add(1)
				return nil
			}).AnyTimes()

			connectInitiatorToDestination(initiatingServerStream, destinationServerStream, tc.messagesBeforeClose, "inbound", log.NewTestLogger())
			fmt.Printf("Expected calls range [%d,%d]\n", minTimes, maxTimes)
			fmt.Printf("Initiating Server Recv Count: %d\n", initiatingServerRecvCount.Load())
			fmt.Printf("Destination Server Sent Count: %d\n", destinationServerSentCount.Load())
			fmt.Printf("Responses count (Not enforced)\n")
			fmt.Printf("Initiating Server Sent Count: %d\n", initiatingServerSentCount.Load())
			fmt.Printf("Destination Server Recv Count: %d\n", destinationServerRecvCount.Load())
		})
	}
}

func (s *adminserviceSuite) TestAddOrUpdateRemoteCluster() {
	var (
		fakeExternalAddr = "fake-external-addr"
		makeOriginalReq  = func() *adminservice.AddOrUpdateRemoteClusterRequest {
			return &adminservice.AddOrUpdateRemoteClusterRequest{
				FrontendAddress:               "fake-original-addr",
				EnableRemoteClusterConnection: true,
			}
		}
		makeModifiedReq = func() *adminservice.AddOrUpdateRemoteClusterRequest {
			return &adminservice.AddOrUpdateRemoteClusterRequest{
				FrontendAddress:               fakeExternalAddr,
				EnableRemoteClusterConnection: true,
			}
		}
		expResp = &adminservice.AddOrUpdateRemoteClusterResponse{}
	)

	cases := []struct {
		name string

		opts        proxyOptions
		reqMetadata map[string]string
		expectedReq *adminservice.AddOrUpdateRemoteClusterRequest
	}{
		{
			name: "no override on outbound request",
			opts: proxyOptions{
				IsInbound: false,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ProxyServerConfig{
							TCPServerSetting: config.TCPServerSetting{
								ExternalAddress: fakeExternalAddr,
							},
						},
					},
				},
			},
			expectedReq: makeOriginalReq(),
		},
		{
			name: "override on inbound request",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ProxyServerConfig{
							TCPServerSetting: config.TCPServerSetting{
								ExternalAddress: fakeExternalAddr,
							},
						},
					},
				},
			},
			expectedReq: makeModifiedReq(), // request is modified
		},
		{
			name: "override on inbound request with translation disabled header",
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ProxyServerConfig{
							TCPServerSetting: config.TCPServerSetting{
								ExternalAddress: fakeExternalAddr,
							},
						},
					},
				},
			},
			expectedReq: makeOriginalReq(), // request is not modified
		},
		{
			name: "no override on empty config",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ProxyServerConfig{
							TCPServerSetting: config.TCPServerSetting{
								ExternalAddress: "", // empty
							},
						},
					},
				},
			},
			expectedReq: makeOriginalReq(),
		},
		{
			name: "nil outbound config",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: nil,
				},
			},
			expectedReq: makeOriginalReq(),
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(c.reqMetadata))
			server := s.newAdminServiceProxyServer(c.opts)
			s.adminClientMock.EXPECT().AddOrUpdateRemoteCluster(ctx, c.expectedReq).Return(expResp, nil)
			resp, err := server.AddOrUpdateRemoteCluster(ctx, makeOriginalReq())
			s.NoError(err)
			s.Equal(expResp, resp)

		})
	}
}

func (s *adminserviceSuite) TestAPIOverrides_FailoverVersionIncrement() {
	req := &adminservice.DescribeClusterRequest{}
	makeResp := func() *adminservice.DescribeClusterResponse {
		return &adminservice.DescribeClusterResponse{
			FailoverVersionIncrement: 1,
		}
	}

	overrideValue := int64(100)
	makeOverrideResp := func() *adminservice.DescribeClusterResponse {
		return &adminservice.DescribeClusterResponse{
			FailoverVersionIncrement: overrideValue,
		}
	}

	createOverrideConfig := func() *config.ProxyConfig {
		return &config.ProxyConfig{
			APIOverrides: &config.APIOverridesConfig{
				AdminSerivce: config.AdminServiceOverrides{
					DescribeCluster: &config.DescribeClusterOverride{
						Response: config.DescribeClusterResponseOverrides{
							FailoverVersionIncrement: &overrideValue,
						},
					},
				},
			},
		}
	}

	cases := []struct {
		name        string
		opts        proxyOptions
		reqMetadata map[string]string
		mockResp    *adminservice.DescribeClusterResponse
		expResp     *adminservice.DescribeClusterResponse
	}{
		{
			name: "nil override config",
			opts: proxyOptions{
				IsInbound: true,
			},
			mockResp: makeResp(),
			expResp:  makeResp(),
		},
		{
			name: "override inbound",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Inbound: createOverrideConfig(),
				},
			},
			mockResp: makeResp(),
			expResp:  makeOverrideResp(),
		},
		{
			name: "override outbound",
			opts: proxyOptions{
				IsInbound: false,
				Config: config.S2SProxyConfig{
					Outbound: createOverrideConfig(),
				},
			},
			mockResp: makeResp(),
			expResp:  makeOverrideResp(),
		},
		{
			name: "override inbound with request translation disabled",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Inbound: createOverrideConfig(),
				},
			},
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			mockResp: makeResp(),
			expResp:  makeResp(),
		},
		{
			name: "override outbound with request translation disabled",
			opts: proxyOptions{
				IsInbound: false,
				Config: config.S2SProxyConfig{
					Outbound: createOverrideConfig(),
				},
			},
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			mockResp: makeResp(),
			expResp:  makeResp(),
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(c.reqMetadata))
			server := s.newAdminServiceProxyServer(c.opts)
			s.adminClientMock.EXPECT().DescribeCluster(ctx, gomock.Any()).Return(c.mockResp, nil)
			resp, err := server.DescribeCluster(ctx, req)
			s.NoError(err)
			s.Equal(c.expResp, resp)
		})
	}
}
