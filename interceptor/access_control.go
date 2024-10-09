package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/status"
	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type (
	AccessControlInterceptor struct {
		logger             log.Logger
		adminServiceAccess *auth.AccessControl
		namespaceAccess    *auth.AccessControl
	}
)

func NewAccessControlInterceptor(
	logger log.Logger,
	aclPolicy *config.ACLPolicy,
) *AccessControlInterceptor {
	var adminServiceAccess *auth.AccessControl
	var namespaceAccess *auth.AccessControl
	if aclPolicy != nil {
		adminServiceAccess = auth.NewAccesControl(aclPolicy.AllowedMethods.AdminService)
		namespaceAccess = auth.NewAccesControl(aclPolicy.AllowedNamespaces)
	}

	return &AccessControlInterceptor{
		logger:             logger,
		adminServiceAccess: adminServiceAccess,
		namespaceAccess:    namespaceAccess,
	}
}

func (i *AccessControlInterceptor) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if i.adminServiceAccess == nil || i.namespaceAccess == nil {
		return handler(ctx, req)
	}

	if i.adminServiceAccess != nil && strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {
		methodName := api.MethodName(info.FullMethod)
		if !i.adminServiceAccess.IsAllowed(methodName) {
			return nil, status.Errorf(codes.PermissionDenied, fmt.Sprintf("Calling method %s is not allowed.", methodName))
		}
	}

	if i.namespaceAccess != nil && strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) ||
		strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {

		notAllowed, err := visitNamespace(req, func(name string) (string, bool) {
			allowed := i.namespaceAccess.IsAllowed(name)
			return name, !allowed
		})

		if notAllowed || err != nil {
			methodName := api.MethodName(info.FullMethod)
			return nil, status.Errorf(codes.PermissionDenied, fmt.Sprintf("Calling method %s is not allowed.", methodName))
		}
	}

	return handler(ctx, req)
}

func (i *AccessControlInterceptor) StreamIntercept(
	service interface{},
	serverStream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	if i.adminServiceAccess != nil && strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {
		methodName := api.MethodName(info.FullMethod)
		if !i.adminServiceAccess.IsAllowed(methodName) {
			return status.Errorf(codes.PermissionDenied, fmt.Sprintf("Calling method %s is not allowed.", methodName))
		}
	}

	return handler(service, serverStream)
}
