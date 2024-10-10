package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

func TestAccessControlInterceptor(t *testing.T) {
	cases := []struct {
		name string

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
