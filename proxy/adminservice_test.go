package proxy

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/adminservicemock/v1"
	"go.temporal.io/server/common/log"

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

func (s *adminserviceSuite) newAdminServiceProxyServer(opts proxyOptions) adminservice.AdminServiceServer {
	cfg := config.ClientConfig{
		ForwardAddress: "fake-forward-address",
		TLS:            encryption.ClientTLSConfig{},
	}
	s.clientFactoryMock.EXPECT().NewRemoteAdminClient(cfg).Return(s.adminClientMock, nil).Times(1)
	return NewAdminServiceProxyServer("test-service-name", cfg, s.clientFactoryMock, opts, log.NewTestLogger())
}

func (s *adminserviceSuite) TestAddOrUpdateRemoteCluster() {
	var (
		fakeExternalAddr = "fake-external-addr"
		originalReq      = &adminservice.AddOrUpdateRemoteClusterRequest{
			FrontendAddress:               "fake-original-addr",
			EnableRemoteClusterConnection: true,
		}
		modifedReq = &adminservice.AddOrUpdateRemoteClusterRequest{
			FrontendAddress:               fakeExternalAddr,
			EnableRemoteClusterConnection: true,
		}
		expResp = &adminservice.AddOrUpdateRemoteClusterResponse{}
	)

	cases := []struct {
		name string

		opts        proxyOptions
		expectedReq *adminservice.AddOrUpdateRemoteClusterRequest
	}{
		{
			name: "no override on outbound request",
			opts: proxyOptions{
				IsInbound: false,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ServerConfig{
							ExternalAddress: fakeExternalAddr,
						},
					},
				},
			},
			expectedReq: originalReq,
		},
		{
			name: "override on inbound request",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ServerConfig{
							ExternalAddress: fakeExternalAddr,
						},
					},
				},
			},
			expectedReq: modifedReq, // request is modified
		},
		{
			name: "no override on empty config",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: &config.ProxyConfig{
						Server: config.ServerConfig{
							ExternalAddress: "", // empty
						},
					},
				},
			},
			expectedReq: originalReq,
		},
		{
			name: "nil outbound config",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Outbound: nil,
				},
			},
			expectedReq: originalReq,
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			ctx := context.Background()
			server := s.newAdminServiceProxyServer(c.opts)
			s.adminClientMock.EXPECT().AddOrUpdateRemoteCluster(ctx, c.expectedReq).Return(expResp, nil)
			resp, err := server.AddOrUpdateRemoteCluster(ctx, originalReq)
			s.NoError(err)
			s.Equal(expResp, resp)

		})
	}
}

func (s *adminserviceSuite) testAccessControl() {
	cases := []struct {
		name string

		opts       proxyOptions
		notAllowed bool
	}{
		{
			name: "no AccessControl",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Inbound: &config.ProxyConfig{},
				},
			},
		},
		{
			name: "With AccessControl Allowed",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Inbound: &config.ProxyConfig{
						ACLPolicy: &config.ACLPolicy{
							AllowedMethods: config.AllowedMethods{
								AdminService: []string{
									"AddOrUpdateRemoteCluster",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "With AccessControl Not Allowed",
			opts: proxyOptions{
				IsInbound: true,
				Config: config.S2SProxyConfig{
					Inbound: &config.ProxyConfig{
						ACLPolicy: &config.ACLPolicy{
							AllowedMethods: config.AllowedMethods{
								AdminService: []string{
									"AddSearchAttributes",
									"GetReplicationMessages",
								},
							},
						},
					},
				},
			},
			notAllowed: true,
		},
	}

	cfg := config.ClientConfig{
		ForwardAddress: "fake-forward-address",
		TLS:            encryption.ClientTLSConfig{},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			ctx := context.Background()
			if !c.notAllowed {
				s.clientFactoryMock.EXPECT().NewRemoteAdminClient(cfg).Return(s.adminClientMock, nil).Times(1)
				s.adminClientMock.EXPECT().AddOrUpdateRemoteCluster(ctx, gomock.Any()).Return(nil, nil)
			}

			server := NewAdminServiceProxyServer("test-service-name", cfg, s.clientFactoryMock, c.opts, log.NewTestLogger())
			_, err := server.AddOrUpdateRemoteCluster(ctx, &adminservice.AddOrUpdateRemoteClusterRequest{})

			if c.notAllowed {
				s.ErrorContains(err, "PermissionDenied")
			} else {
				s.NoError(err)
			}
		})
	}
}
