package interceptor

import (
	"context"
	"strings"

	"github.com/gogo/status"
	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
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

func createNamespaceAccessControl(access *auth.AccessControl) matcher {
	return func(name string) (string, bool) {
		var notAllowed bool
		if access != nil {
			notAllowed = !access.IsAllowed(name)
		}

		return name, notAllowed
	}
}

func isNamespaceAccessAllowed(obj any, access *auth.AccessControl) (bool, error) {
	notAllowed, err := visitNamespace(obj, createNamespaceAccessControl(access))
	if err != nil {
		return false, err
	}

	return !notAllowed, nil
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
			return nil, status.Errorf(codes.PermissionDenied, "Calling method %s is not allowed.", methodName)
		}
	}

	if i.namespaceAccess != nil && strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) ||
		strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {
		allowed, err := isNamespaceAccessAllowed(req, i.namespaceAccess)
		if !allowed || err != nil {
			methodName := api.MethodName(info.FullMethod)
			if err != nil {
				logger := log.With(
					i.logger,
					tag.NewStringTag("method", methodName),
					tag.NewAnyTag("obj", req),
				)

				logger.Error("namespace access control error", tag.Error(err))
			}

			return nil, status.Errorf(codes.PermissionDenied, "Calling method %s is not allowed.", methodName)
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
			return status.Errorf(codes.PermissionDenied, "Calling method %s is not allowed.", methodName)
		}
	}

	return handler(service, serverStream)
}
