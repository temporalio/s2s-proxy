package proxy

import (
	"context"
	"slices"
	"testing"

	gomockold "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/api/workflowservicemock/v1"
	"go.temporal.io/server/common/log"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	clientmock "github.com/temporalio/s2s-proxy/mocks/client"
)

var (
	listNamespacesResponse = &workflowservice.ListNamespacesResponse{
		Namespaces: []*workflowservice.DescribeNamespaceResponse{
			{
				NamespaceInfo: &namespace.NamespaceInfo{
					Name:        "Bob Ross's Paint Shop",
					State:       enums.NAMESPACE_STATE_REGISTERED,
					Description: "Happy Little Trees aplenty",
					OwnerEmail:  "bob-ross@happy-accidents.com",
					Data:        nil,
					Id:          "abcd-efgh-ijkl-mnop",
					Capabilities: &namespace.NamespaceInfo_Capabilities{
						EagerWorkflowStart: true,
						SyncUpdate:         true,
						AsyncUpdate:        true,
					},
					SupportsSchedules: true,
				},
			},
			{
				NamespaceInfo: &namespace.NamespaceInfo{
					Name:        "Steve's Auto Repair",
					State:       enums.NAMESPACE_STATE_REGISTERED,
					Description: "It's not in Bob's cluster",
					OwnerEmail:  "steve@sad-accidents.com",
					Data:        nil,
					Id:          "qrst-uvwx-yzab-cdef",
					Capabilities: &namespace.NamespaceInfo_Capabilities{
						EagerWorkflowStart: true,
						SyncUpdate:         true,
						AsyncUpdate:        true,
					},
					SupportsSchedules: true,
				},
			},
		},
	}
)

// Simple test for the workflow client filtering: This mocks out the underlying WorkflowServiceClient and returns
// a predefined set of namespaces to the proxy layer. Then we check to make sure the proxy kept the allowed namespace
// and rejected the disallowed namespace.
func TestNamespaceFiltering(t *testing.T) {
	clientFactoryController := gomock.NewController(t)
	// workflowservicemock is still using golang-mock and not uber-mock, so we get to have two controllers here
	wfServiceController := gomockold.NewController(t)
	mockServiceClient := workflowservicemock.NewMockWorkflowServiceClient(wfServiceController)
	mockServiceClient.EXPECT().ListNamespaces(gomock.Any(), gomock.Any()).Return(listNamespacesResponse, nil)

	mockClientFactory := clientmock.NewMockClientFactory(clientFactoryController)
	clientConfig := config.ProxyClientConfig{
		TCPClientSetting: config.TCPClientSetting{
			ServerAddress: "fake-forward-address",
			TLS:           encryption.ClientTLSConfig{},
		},
	}
	mockClientFactory.EXPECT().NewRemoteWorkflowServiceClient(clientConfig).Return(mockServiceClient, nil).Times(1)
	wfProxy := NewWorkflowServiceProxyServer("My cool test server", clientConfig, mockClientFactory,
		auth.NewAccesControl([]string{"Bob Ross's Paint Shop"}), log.NewTestLogger())

	res, _ := wfProxy.ListNamespaces(metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{})),
		&workflowservice.ListNamespacesRequest{
			PageSize:        10,
			NextPageToken:   nil,
			NamespaceFilter: &namespace.NamespaceFilter{IncludeDeleted: true},
		})

	assert.NotEqualf(t, -1, slices.IndexFunc(res.Namespaces, func(ns *workflowservice.DescribeNamespaceResponse) bool {
		return ns.NamespaceInfo.Name == "Bob Ross's Paint Shop"
	}), "Couldn't find \"%s\" in the response! It should have matched the ACL.\nResponse: %s", "Bob Ross's Paint Shop", res.Namespaces)
	steveIndex := slices.IndexFunc(res.Namespaces, func(ns *workflowservice.DescribeNamespaceResponse) bool {
		return ns.NamespaceInfo.Name == "Steve's Auto Repair"
	})
	var matchingNS *namespace.NamespaceInfo
	if steveIndex > 0 {
		matchingNS = res.Namespaces[steveIndex].NamespaceInfo
	}
	assert.Equalf(t, -1, steveIndex, "Shouldn't have found namespace %s\n in list response: %s", matchingNS, res.Namespaces)
}
