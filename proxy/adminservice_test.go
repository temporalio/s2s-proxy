package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	namespacev1 "go.temporal.io/api/namespace/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/adminservicemock/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	replicationv1 "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
)

func TestAdminserviceSuite(t *testing.T) {
	suite.Run(t, new(adminserviceSuite))
}

type adminserviceSuite struct {
	suite.Suite
	ctrl *gomock.Controller

	adminClientMock *adminservicemock.MockAdminServiceClient
}

func (s *adminserviceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.adminClientMock = adminservicemock.NewMockAdminServiceClient(s.ctrl)
}

func (s *adminserviceSuite) AfterTest() {
	s.ctrl.Finish()
}

type adminProxyServerInput struct {
	overrides    AdminServiceOverrides
	metricLabels []string
}

func (s *adminserviceSuite) newAdminServiceProxyServer(in adminProxyServerInput, observer *ReplicationStreamObserver) adminservice.AdminServiceServer {
	return NewAdminServiceProxyServer("test-service-name", s.adminClientMock,
		s.adminClientMock, in.overrides, in.metricLabels, observer.ReportStreamValue, config.ShardCountConfig{}, LCMParameters{},
		RoutingParameters{}, logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{})), nil, context.Background())
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
				metricLabels: []string{"outbound"},
			},
			expectedReq: makeOriginalReq(),
		},
		{
			name: "override on inbound request",
			adminProxyServerInput: adminProxyServerInput{
				overrides:    AdminServiceOverrides{ReplicationEndpoint: fakeExternalAddr},
				metricLabels: []string{"inbound"},
			},
			expectedReq: makeModifiedReq(), // request is modified
		},
		{
			name: "override on inbound request with translation disabled header",
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			adminProxyServerInput: adminProxyServerInput{
				overrides:    AdminServiceOverrides{ReplicationEndpoint: fakeExternalAddr},
				metricLabels: []string{"inbound"},
			},
			expectedReq: makeOriginalReq(), // request is not modified
		},
		{
			name: "no override on empty config",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
			},
			expectedReq: makeOriginalReq(),
		},
		{
			name: "nil outbound config",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"outbound"},
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
			s.True(proto.Equal(expResp, resp))
			s.Equal("[]", observer.PrintActiveStreams())
		})
	}
}

func (s *adminserviceSuite) TestAPIOverrides_GetNamespaceReplicationMessages() {
	req := &adminservice.GetNamespaceReplicationMessagesRequest{ClusterName: "test-cluster"}

	customSAAliases := func(mappings ...config.CustomSAAliasNamespaceMapping) config.CustomSAAliasConfig {
		return config.CustomSAAliasConfig{NamespaceMappings: mappings}
	}
	namespace1Mapping := config.CustomSAAliasNamespaceMapping{
		Name: "namespace1",
		CustomSearchAttributeAliases: map[string]string{
			"Keyword01": "MyKeyword",
			"Text01":    "MyText",
		},
	}
	// What the handler is expected to write onto the task.
	namespace1Aliases := map[string]string{
		"Keyword01": "MyKeyword",
		"Text01":    "MyText",
	}

	// makeNSTask builds a namespace replication task. A nil aliases map leaves
	// Config.CustomSearchAttributeAliases unset; use makeNSTaskNilConfig for a nil Config.
	makeNSTask := func(ns string, aliases map[string]string) *replicationv1.ReplicationTask {
		return &replicationv1.ReplicationTask{
			TaskType: enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK,
			Attributes: &replicationv1.ReplicationTask_NamespaceTaskAttributes{
				NamespaceTaskAttributes: &replicationv1.NamespaceTaskAttributes{
					Id:   "task-id-" + ns,
					Info: &namespacev1.NamespaceInfo{Name: ns},
					Config: &namespacev1.NamespaceConfig{
						CustomSearchAttributeAliases: aliases,
					},
				},
			},
		}
	}
	makeNSTaskNilConfig := func(ns string) *replicationv1.ReplicationTask {
		return &replicationv1.ReplicationTask{
			TaskType: enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK,
			Attributes: &replicationv1.ReplicationTask_NamespaceTaskAttributes{
				NamespaceTaskAttributes: &replicationv1.NamespaceTaskAttributes{
					Id:   "task-id-" + ns,
					Info: &namespacev1.NamespaceInfo{Name: ns},
				},
			},
		}
	}
	makeResp := func(tasks ...*replicationv1.ReplicationTask) *adminservice.GetNamespaceReplicationMessagesResponse {
		return &adminservice.GetNamespaceReplicationMessagesResponse{
			Messages: &replicationv1.ReplicationMessages{
				ReplicationTasks:       tasks,
				LastRetrievedMessageId: 1234,
			},
		}
	}

	cases := []struct {
		name                  string
		reqMetadata           map[string]string
		adminProxyServerInput adminProxyServerInput
		mockResp              *adminservice.GetNamespaceReplicationMessagesResponse
		expResp               *adminservice.GetNamespaceReplicationMessagesResponse
	}{
		{
			name: "no override config",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
			},
			mockResp: makeResp(makeNSTask("namespace1", nil)),
			expResp:  makeResp(makeNSTask("namespace1", nil)),
		},
		{
			name: "override matching namespace",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: makeResp(makeNSTask("namespace1", nil)),
			expResp:  makeResp(makeNSTask("namespace1", namespace1Aliases)),
		},
		{
			name: "replaces pre-existing aliases",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: makeResp(makeNSTask("namespace1", map[string]string{
				"Keyword01": "SourceKeyword",
				"Bool01":    "SourceBool",
			})),
			expResp: makeResp(makeNSTask("namespace1", namespace1Aliases)),
		},
		{
			// Only the configured namespace is touched; others pass through untouched.
			name: "mixed batch overrides only configured namespaces",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: makeResp(
				makeNSTask("namespace1", nil),
				makeNSTask("namespace2", map[string]string{"Keyword01": "UntouchedKeyword"}),
			),
			expResp: makeResp(
				makeNSTask("namespace1", namespace1Aliases),
				makeNSTask("namespace2", map[string]string{"Keyword01": "UntouchedKeyword"}),
			),
		},
		{
			name: "unconfigured namespace",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: makeResp(makeNSTask("unknownNamespace", nil)),
			expResp:  makeResp(makeNSTask("unknownNamespace", nil)),
		},
		{
			name: "namespace configured with no aliases",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides: AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping, config.CustomSAAliasNamespaceMapping{
					Name:                         "namespace2",
					CustomSearchAttributeAliases: map[string]string{},
				})},
			},
			mockResp: makeResp(makeNSTask("namespace2", nil)),
			expResp:  makeResp(makeNSTask("namespace2", nil)),
		},
		{
			// Current behavior: a task with no Config is skipped, so the namespace
			// replicates with no aliases at all rather than the configured ones.
			name: "task with nil config is skipped",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: makeResp(makeNSTaskNilConfig("namespace1")),
			expResp:  makeResp(makeNSTaskNilConfig("namespace1")),
		},
		{
			name: "non-namespace tasks and nil entries",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: makeResp(
				nil,
				&replicationv1.ReplicationTask{TaskType: enumsspb.REPLICATION_TASK_TYPE_HISTORY_V2_TASK},
				makeNSTask("namespace1", nil),
			),
			expResp: makeResp(
				nil,
				&replicationv1.ReplicationTask{TaskType: enumsspb.REPLICATION_TASK_TYPE_HISTORY_V2_TASK},
				makeNSTask("namespace1", namespace1Aliases),
			),
		},
		{
			name: "nil messages",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			mockResp: &adminservice.GetNamespaceReplicationMessagesResponse{},
			expResp:  &adminservice.GetNamespaceReplicationMessagesResponse{},
		},
		{
			// Unlike the FVI and ReplicationEndpoint overrides, this one does not honor
			// the disable-translation header.
			name: "override applied with request translation disabled",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{CustomSearchAttributeAliases: customSAAliases(namespace1Mapping)},
			},
			reqMetadata: map[string]string{
				common.RequestTranslationHeaderName: "false",
			},
			mockResp: makeResp(makeNSTask("namespace1", nil)),
			expResp:  makeResp(makeNSTask("namespace1", namespace1Aliases)),
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(c.reqMetadata))
			observer := NewReplicationStreamObserver(log.NewTestLogger())
			server := s.newAdminServiceProxyServer(c.adminProxyServerInput, observer)
			s.adminClientMock.EXPECT().GetNamespaceReplicationMessages(ctx, req).Return(c.mockResp, nil)
			resp, err := server.GetNamespaceReplicationMessages(ctx, req)
			s.NoError(err)
			s.True(proto.Equal(c.expResp, resp), "expected %v, got %v", c.expResp, resp)
			s.Equal("[]", observer.PrintActiveStreams())
		})
	}
}

func (s *adminserviceSuite) TestAPIOverrides_GetNamespaceReplicationMessagesError() {
	req := &adminservice.GetNamespaceReplicationMessagesRequest{ClusterName: "test-cluster"}
	expErr := errors.New("some error")

	observer := NewReplicationStreamObserver(log.NewTestLogger())
	server := s.newAdminServiceProxyServer(adminProxyServerInput{
		metricLabels: []string{"inbound"},
		overrides: AdminServiceOverrides{CustomSearchAttributeAliases: config.CustomSAAliasConfig{
			NamespaceMappings: []config.CustomSAAliasNamespaceMapping{
				{
					Name:                         "namespace1",
					CustomSearchAttributeAliases: map[string]string{"Keyword01": "MyKeyword"},
				},
			},
		}},
	}, observer)
	s.adminClientMock.EXPECT().GetNamespaceReplicationMessages(gomock.Any(), req).Return(nil, expErr)

	resp, err := server.GetNamespaceReplicationMessages(context.Background(), req)
	s.ErrorIs(err, expErr)
	s.Nil(resp)
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
				metricLabels: []string{"outbound"},
			},
			mockResp: makeResp(),
			expResp:  makeResp(),
		},
		{
			name: "override inbound",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{FVI: overrideValue},
			},
			mockResp: makeResp(),
			expResp:  makeOverrideResp(),
		},
		{
			name: "override outbound",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"outbound"},
				overrides:    AdminServiceOverrides{FVI: overrideValue},
			},
			mockResp: makeResp(),
			expResp:  makeOverrideResp(),
		},
		{
			name: "override inbound with request translation disabled",
			adminProxyServerInput: adminProxyServerInput{
				metricLabels: []string{"inbound"},
				overrides:    AdminServiceOverrides{FVI: overrideValue},
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
				overrides:    AdminServiceOverrides{FVI: overrideValue},
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
			s.True(proto.Equal(c.expResp, resp))
			s.Equal("[]", observer.PrintActiveStreams())
		})
	}
}
