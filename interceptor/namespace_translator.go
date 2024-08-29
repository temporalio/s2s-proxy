package interceptor

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

const (
	// Currently, name translation relies on reflection to recursively find namespace name fields
	// in request and response objects. This is the default max depth allowed for that recursion
	// to protect against circular pointers or etc. It is configurable.
	defaultReflectionRecursionMaxDepth int = 20
)

type (
	NamespaceNameTranslator struct {
		logger                      log.Logger
		localToRemote               map[string]string
		remoteToLocal               map[string]string
		reflectionRecursionMaxDepth int
	}
)

func NewNamespaceNameTranslator(
	logger log.Logger,
	cfg config.ProxyConfig,
	isInbound bool,
) *NamespaceNameTranslator {
	localToRemote := map[string]string{}
	remoteToLocal := map[string]string{}
	for _, tr := range cfg.NamespaceNameTranslation.Mappings {
		localToRemote[tr.LocalName] = tr.RemoteName
		remoteToLocal[tr.RemoteName] = tr.LocalName
	}
	if isInbound {
		// Invert the mapping for requests to the inbound listener.
		localToRemote, remoteToLocal = remoteToLocal, localToRemote
	}

	return &NamespaceNameTranslator{
		logger:                      logger,
		localToRemote:               localToRemote,
		remoteToLocal:               remoteToLocal,
		reflectionRecursionMaxDepth: cfg.NamespaceNameTranslation.ReflectionRecursionMaxDepth,
	}
}

var _ grpc.UnaryServerInterceptor = (*NamespaceNameTranslator)(nil).Intercept

func (i *NamespaceNameTranslator) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if len(i.localToRemote) == 0 {
		return handler(ctx, req)
	}

	methodName := api.MethodName(info.FullMethod)
	if strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) {
		i.logger.Debug("intercepted workflowservice request", tag.NewStringTag("method", methodName))

		// Translate local namespace name to remote namespace name before sending the request.
		changed, trErr := translateNamespace(req, i.localToRemote, i.reflectionRecursionMaxDepth)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		// Translate remote namespace name to local namespace name in the response.
		changed, trErr = translateNamespace(resp, i.remoteToLocal, i.reflectionRecursionMaxDepth)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Response", resp)
		return resp, err
	} else if strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {
		i.logger.Debug("intercepted adminservice request", tag.NewStringTag("method", methodName))

		resp, err := handler(ctx, req)
		if resp == nil {
			return resp, err
		}

		// Translate the namespace name in GetNamespaceReplicationMessagesResponse.
		// We change the remote namespace name to local name.
		switch rt := resp.(type) {
		case *adminservice.GetNamespaceReplicationMessagesResponse:
			if rt == nil || rt.Messages == nil {
				return resp, err
			}
			for _, task := range rt.Messages.ReplicationTasks {
				switch attr := task.Attributes.(type) {
				case *replicationspb.ReplicationTask_NamespaceTaskAttributes:
					if attr == nil || attr.NamespaceTaskAttributes == nil || attr.NamespaceTaskAttributes.Info == nil {
						continue
					}
					oldName := attr.NamespaceTaskAttributes.Info.Name
					newName, found := i.remoteToLocal[oldName]
					if found {
						attr.NamespaceTaskAttributes.Info.Name = newName
					}
					logTranslateNamespaceResult(i.logger, found, nil, methodName+"Response", resp)
				}
			}
		}
		return resp, err
	} else {
		return handler(ctx, req)
	}
}

func translateNamespace(obj any, mapping map[string]string, maxDepth int) (bool, error) {
	if maxDepth <= 0 {
		maxDepth = defaultReflectionRecursionMaxDepth
	}
	val := reflect.ValueOf(obj)
	return translateNamespaceRecursive(val, mapping, 0, maxDepth)
}

func translateNamespaceRecursive(val reflect.Value, mapping map[string]string, depth, maxDepth int) (bool, error) {
	if depth > maxDepth {
		// Protect against potential circular pointer.
		return false, fmt.Errorf("translateNamespaceRecursive max depth reached")
	}

	var changed bool

	switch val.Kind() {
	case reflect.Ptr, reflect.Interface:
		c, err := translateNamespaceRecursive(val.Elem(), mapping, depth+1, maxDepth)
		changed = changed || c
		if err != nil {
			return changed, err
		}
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)

			fieldType := val.Type().Field(i)
			if !fieldType.IsExported() {
				continue
			}

			if field.Kind() != reflect.String {
				c, err := translateNamespaceRecursive(field, mapping, depth+1, maxDepth)
				changed = changed || c
				if err != nil {
					return changed, err
				}
			} else {
				for _, nsFieldName := range []string{
					"Namespace",
					"WorkflowNamespace", // PollActivityTaskQueueResponse
				} {
					if fieldType.Name != nsFieldName {
						continue
					}

					old := field.String()
					new, ok := mapping[old]
					if !ok {
						continue
					}
					field.SetString(new)
					if old != new {
						changed = true
					}
				}
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			el := val.Index(i)
			c, err := translateNamespaceRecursive(el, mapping, depth+1, maxDepth)
			changed = changed || c
			if err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

func logTranslateNamespaceResult(logger log.Logger, changed bool, err error, methodName string, obj any) {
	logger = log.With(
		logger,
		tag.NewStringTag("method", methodName),
		tag.NewAnyTag("obj", obj),
	)
	if err != nil {
		logger.Error("namespace translation error", tag.NewErrorTag(err))
	} else if changed {
		logger.Debug("namespace translation applied")
	} else {
		logger.Debug("namespace translation not applied")
	}
}
