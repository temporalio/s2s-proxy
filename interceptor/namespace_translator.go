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

type (
	NamespaceNameTranslator struct {
		logger log.Logger

		localToRemote map[string]string
		remoteToLocal map[string]string
	}
)

func NewNamespaceNameTranslator(
	logger log.Logger,
	configProvider config.ConfigProvider,
) (*NamespaceNameTranslator, error) {
	cfg := configProvider.GetS2SProxyConfig().NamespaceTranslation

	localToRemote := map[string]string{}
	remoteToLocal := map[string]string{}
	for _, tr := range cfg {
		if tr.LocalName == "" || tr.RemoteName == "" {
			return nil, fmt.Errorf("invalid namespace translation config: localName=%q remoteName=%q",
				tr.LocalName, tr.RemoteName)
		}
		localToRemote[tr.LocalName] = tr.RemoteName
		remoteToLocal[tr.RemoteName] = tr.LocalName
	}

	return &NamespaceNameTranslator{
		logger:        logger,
		localToRemote: localToRemote,
		remoteToLocal: remoteToLocal,
	}, nil
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
		// Translate local namespace name to remote namespace name before sending the request.
		changed, trErr := translateNamespace(req, i.localToRemote)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		// Translate remote namespace name to local namespace name in the response.
		changed, trErr = translateNamespace(resp, i.remoteToLocal)
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

func translateNamespace(req any, mapping map[string]string) (bool, error) {
	val := reflect.ValueOf(req)
	return translateNamespaceRecursive(val, mapping, 0)
}

func translateNamespaceRecursive(val reflect.Value, mapping map[string]string, depth int) (bool, error) {
	if depth > 20 {
		// Protect against potential circular pointer.
		return false, fmt.Errorf("translateNamespaceRecursive max depth reached (circular pointers in type?)")
	}

	var changed bool

	switch val.Kind() {
	case reflect.Ptr, reflect.Interface:
		c, err := translateNamespaceRecursive(val.Elem(), mapping, depth+1)
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
				c, err := translateNamespaceRecursive(field, mapping, depth+1)
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
			c, err := translateNamespaceRecursive(el, mapping, depth+1)
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
