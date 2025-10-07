package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/adminservicemock/v1"
	"go.temporal.io/server/common/log"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
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

type adminProxyServerInput struct {
	outboundAddressOverride string
	apiOverrides            *config.APIOverridesConfig
	metricLabels            []string
}

func (s *adminserviceSuite) newAdminServiceProxyServer(in adminProxyServerInput, observer *ReplicationStreamObserver) adminservice.AdminServiceServer {
	return NewAdminServiceProxyServer("test-service-name", s.adminClientMock, in.outboundAddressOverride,
		in.apiOverrides, in.metricLabels, observer.ReportStreamValue, log.NewTestLogger())
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
		name                  string
		reqMetadata           map[string]string
		expectedReq           *adminservice.AddOrUpdateRemoteClusterRequest
		adminProxyServerInput adminProxyServerInput
	}{
		{
			name: "no override on outbound request",
			adminProxyServerInput: adminProxyServerInput{
				outboundAddressOverride: "",
				metricLabels:            []string{"outbound"},
			},
			expectedReq: makeOriginalReq(),
		},
		{
			name: "override on inbound request",
			adminProxyServerInput: adminProxyServerInput{
				outboundAddressOverride: fakeExternalAddr,
				metricLabels:            []string{"inbound"},
			},
			expectedReq: makeModifiedReq(), // request is modified
		},
		{
			name: "override on inbound request with translation disabled header",
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			adminProxyServerInput: adminProxyServerInput{
				outboundAddressOverride: fakeExternalAddr,
				metricLabels:            []string{"inbound"},
			},
			expectedReq: makeOriginalReq(), // request is not modified
		},
		{
			name: "no override on empty config",
			adminProxyServerInput: adminProxyServerInput{
				outboundAddressOverride: "",
				metricLabels:            []string{"inbound"},
			},
			expectedReq: makeOriginalReq(),
		},
		{
			name: "nil outbound config",
			adminProxyServerInput: adminProxyServerInput{
				outboundAddressOverride: "",
				metricLabels:            []string{"outbound"},
			},
			expectedReq: makeOriginalReq(),
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(c.reqMetadata))
			observer := NewReplicationStreamObserver(log.NewTestLogger())
			server := s.newAdminServiceProxyServer(c.adminProxyServerInput, observer)
			s.adminClientMock.EXPECT().AddOrUpdateRemoteCluster(ctx, c.expectedReq).Return(expResp, nil)
			resp, err := server.AddOrUpdateRemoteCluster(ctx, makeOriginalReq())
			s.NoError(err)
			s.Equal(expResp, resp)
			s.Equal("[]", observer.PrintActiveStreams())
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

	createOverrideConfig := func() *config.APIOverridesConfig {
		return &config.APIOverridesConfig{
			AdminSerivce: config.AdminServiceOverrides{
				DescribeCluster: &config.DescribeClusterOverride{
					Response: config.DescribeClusterResponseOverrides{
						FailoverVersionIncrement: &overrideValue,
					},
				},
			},
		}
	}

	cases := []struct {
		name                  string
		reqMetadata           map[string]string
		adminProxyServerInput adminProxyServerInput
		mockResp              *adminservice.DescribeClusterResponse
		expResp               *adminservice.DescribeClusterResponse
	}{
		{
			name: "nil override config",
			adminProxyServerInput: adminProxyServerInput{
				outboundAddressOverride: "",
				metricLabels:            []string{"outbound"},
			},
			mockResp: makeResp(),
			expResp:  makeResp(),
		},
		{
			name: "override inbound",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				apiOverrides: createOverrideConfig(),
			},
			mockResp: makeResp(),
			expResp:  makeOverrideResp(),
		},
		{
			name: "override outbound",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"outbound"},
				apiOverrides: createOverrideConfig(),
			},
			mockResp: makeResp(),
			expResp:  makeOverrideResp(),
		},
		{
			name: "override inbound with request translation disabled",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				apiOverrides: createOverrideConfig(),
			},
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			mockResp: makeResp(),
			expResp:  makeResp(),
		},
		{
			name: "override outbound with request translation disabled",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"outbound"},
				apiOverrides: createOverrideConfig(),
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
			observer := NewReplicationStreamObserver(log.NewTestLogger())
			server := s.newAdminServiceProxyServer(c.adminProxyServerInput, observer)
			s.adminClientMock.EXPECT().DescribeCluster(ctx, gomock.Any()).Return(c.mockResp, nil)
			resp, err := server.DescribeCluster(ctx, req)
			s.NoError(err)
			s.Equal(c.expResp, resp)
			s.Equal("[]", observer.PrintActiveStreams())
		})
	}
}
