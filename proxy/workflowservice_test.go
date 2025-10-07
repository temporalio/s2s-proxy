package proxy

import (
	"context"
	"slices"
	"testing"

	gomockold "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/api/workflowservicemock/v1"
	"go.temporal.io/server/common/log"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/auth"
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

type workflowServiceTestSuite struct {
	suite.Suite

	ctrl       *gomock.Controller
	ctrlold    *gomockold.Controller
	clientMock *workflowservicemock.MockWorkflowServiceClient
}

func TestWorkflowService(t *testing.T) {
	suite.Run(t, new(workflowServiceTestSuite))
}

func (s *workflowServiceTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.ctrlold = gomockold.NewController(s.T())
	s.clientMock = workflowservicemock.NewMockWorkflowServiceClient(s.ctrlold)
}

// Simple test for the workflow client filtering: This mocks out the underlying WorkflowServiceClient and returns
// a predefined set of namespaces to the proxy layer. Then we check to make sure the proxy kept the allowed namespace
// and rejected the disallowed namespace.
func (s *workflowServiceTestSuite) TestNamespaceFiltering() {
	wfProxy := NewWorkflowServiceProxyServer("My cool test server", s.clientMock,
		auth.NewAccesControl([]string{"Bob Ross's Paint Shop"}), log.NewTestLogger())

	s.clientMock.EXPECT().ListNamespaces(gomock.Any(), gomock.Any()).Return(listNamespacesResponse, nil)
	res, _ := wfProxy.ListNamespaces(metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{})),
		&workflowservice.ListNamespacesRequest{
			PageSize:        10,
			NextPageToken:   nil,
			NamespaceFilter: &namespace.NamespaceFilter{IncludeDeleted: true},
		})

	s.NotEqualf(-1, slices.IndexFunc(res.Namespaces, func(ns *workflowservice.DescribeNamespaceResponse) bool {
		return ns.NamespaceInfo.Name == "Bob Ross's Paint Shop"
	}), "Couldn't find \"%s\" in the response! It should have matched the ACL.\nResponse: %s", "Bob Ross's Paint Shop", res.Namespaces)
	steveIndex := slices.IndexFunc(res.Namespaces, func(ns *workflowservice.DescribeNamespaceResponse) bool {
		return ns.NamespaceInfo.Name == "Steve's Auto Repair"
	})
	var matchingNS *namespace.NamespaceInfo
	if steveIndex > 0 {
		matchingNS = res.Namespaces[steveIndex].NamespaceInfo
	}
	s.Equalf(-1, steveIndex, "Shouldn't have found namespace %s\n in list response: %s", matchingNS, res.Namespaces)
}

func (s *workflowServiceTestSuite) TestPreserveRedirectionHeader() {
	wfProxy := NewWorkflowServiceProxyServer("My cool test server", s.clientMock, nil, log.NewTestLogger())

	// Client should be called with xdc-redirection=false header
	for _, headerValue := range []string{"true", "false", ""} {
		s.clientMock.EXPECT().DescribeWorkflowExecution(gomockold.Any(), gomockold.Any()).DoAndReturn(
			func(ctx context.Context, request *workflowservice.DescribeWorkflowExecutionRequest, opts ...grpc.CallOption) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
				md, ok := metadata.FromOutgoingContext(ctx)
				s.True(ok)
				s.Equal(md.Get(DCRedirectionContextHeaderName), []string{headerValue})
				return &workflowservice.DescribeWorkflowExecutionResponse{}, nil
			},
		).Times(1)

		// API is passed xdc-redirection=false header
		ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{DCRedirectionContextHeaderName: headerValue}))
		res, err := wfProxy.DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{})
		s.NoError(err)
		s.NotNil(res)
	}
}
