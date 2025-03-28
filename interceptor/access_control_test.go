package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

func TestMethodAccessControlInterceptor(t *testing.T) {
	cases := []struct {
		name       string
		policy     *config.ACLPolicy
		notAllowed bool
	}{
		{
			name: "no AccessControl",
		},
		{
			name: "With AccessControl Allowed",
			policy: &config.ACLPolicy{
				AllowedMethods: config.AllowedMethods{
					AdminService: []string{
						"UnaryAllowedMethod",
						"StreamAllowedMethod",
					},
				},
			},
		},
		{
			name: "With AccessControl Not Allowed",
			policy: &config.ACLPolicy{
				AllowedMethods: config.AllowedMethods{
					AdminService: []string{
						"NotAllowedMethod",
					},
				},
			},
			notAllowed: true,
		},
	}

	logger := log.NewTestLogger()
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: api.AdminServicePrefix + "/" + "UnaryAllowedMethod",
	}

	streamHandler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	streamInfo := &grpc.StreamServerInfo{
		FullMethod: api.AdminServicePrefix + "/" + "StreamAllowedMethod",
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i := NewAccessControlInterceptor(logger, c.policy)
			_, err := i.Intercept(context.Background(), nil, unaryInfo, unaryHandler)
			if c.notAllowed {
				require.ErrorContains(t, err, "PermissionDenied")
			} else {
				require.NoError(t, err)
			}

			err = i.StreamIntercept(nil, nil, streamInfo, streamHandler)
			if c.notAllowed {
				require.ErrorContains(t, err, "PermissionDenied")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWorkflowActionAllowedForForwarding(t *testing.T) {
	cases := []struct {
		methodName string
		expAllowed bool
	}{
		{
			methodName: "StartWorkflowExecution",
			expAllowed: true,
		},
		{
			methodName: "ListNamespaces",
			expAllowed: false,
		},
	}

	logger := log.NewTestLogger()
	i := NewAccessControlInterceptor(logger, nil)

	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	for _, c := range cases {
		t.Run(c.methodName, func(t *testing.T) {
			unaryInfo := &grpc.UnaryServerInfo{
				FullMethod: api.WorkflowServicePrefix + "/" + c.methodName,
			}

			_, err := i.Intercept(context.Background(), nil, unaryInfo, unaryHandler)
			if !c.expAllowed {
				require.ErrorContains(t, err, "PermissionDenied")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func testNamespaceAccessControl(t *testing.T, objCases []objCase) {
	testcases := []struct {
		testName    string
		inputNSName string
		access      *auth.AccessControl
		expAllowed  bool
	}{
		{
			testName:    "nil AccessControl",
			inputNSName: "AllowedNamespace",
			expAllowed:  true,
		},
		{
			testName:    "allowed namespace",
			inputNSName: "AllowedNamespace",
			access:      auth.NewAccesControl([]string{"AllowedNamespace"}),
			expAllowed:  true,
		},
		{
			testName:    "not allowed Namespace",
			inputNSName: "NotAllowedNamespace",
			access:      auth.NewAccesControl([]string{"AllowedNamespace"}),
			expAllowed:  false,
		},
	}

	for _, c := range objCases {
		t.Run(c.objName, func(t *testing.T) {
			for _, ts := range testcases {
				t.Run(ts.testName, func(t *testing.T) {
					input := c.makeType(ts.inputNSName)
					allowed, err := isNamespaceAccessAllowed(input, ts.access)
					if len(c.expError) != 0 {
						require.ErrorContains(t, err, c.expError)
					} else {
						require.NoError(t, err)
						if c.containsNamespace {
							require.Equal(t, ts.expAllowed, allowed)
						} else {
							require.True(t, allowed)
						}

						require.Equal(t, c.makeType(ts.inputNSName), input)
					}
				})
			}
		})
	}
}

func TestNamespaceAccessControl(t *testing.T) {
	testNamespaceAccessControl(t, generateNamespaceObjCases(t))
	testNamespaceAccessControl(t, generateNamespaceReplicationMessages())
}
