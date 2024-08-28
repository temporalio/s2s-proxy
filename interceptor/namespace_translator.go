package interceptor

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/temporalio/s2s-proxy/config"
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
) *NamespaceNameTranslator {
	localToRemote := map[string]string{}
	remoteToLocal := map[string]string{}
	for _, tr := range cfg.NamespaceNameTranslation.Mappings {
		localToRemote[tr.LocalName] = tr.RemoteName
		remoteToLocal[tr.RemoteName] = tr.LocalName
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
		i.logger.Debug("intercepted workflowservice request",
			tag.NewStringTag("method", info.FullMethod),
		)
		// Translate local namespace name to remote namespace name before sending the request.
		changed, trErr := translateNamespace(req, i.localToRemote, i.reflectionRecursionMaxDepth)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		// Translate remote namespace name to local namespace name in the response.
		changed, trErr = translateNamespace(resp, i.remoteToLocal, i.reflectionRecursionMaxDepth)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Response", resp)
		return resp, err
	} else if strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {
		i.logger.Debug("intercepted adminservice request",
			tag.NewStringTag("method", info.FullMethod),
		)
		// TODO: Modify namespace sync message.
		return handler(ctx, req)
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
