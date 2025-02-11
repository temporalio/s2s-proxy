package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/server/api/adminservice/v1"
	repication "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/persistence/serialization"
	"google.golang.org/grpc"
)

type (
	NamespaceNameTranslator struct {
		logger              log.Logger
		requestNameMapping  map[string]string
		responseNameMapping map[string]string
	}
)

func NewNamespaceNameTranslator(
	logger log.Logger,
	cfg config.ProxyConfig,
	isInbound bool,
	nameTranslations config.NamespaceNameTranslationConfig,
) *NamespaceNameTranslator {
	requestNameMapping := map[string]string{}
	responseNameMapping := map[string]string{}
	for _, tr := range nameTranslations.Mappings {
		if isInbound {
			// For inbound listener,
			//   - incoming requests from remote server are modifed to match local server
			//   - outgoing responses to local server are modified to match remote server
			requestNameMapping[tr.RemoteName] = tr.LocalName
			responseNameMapping[tr.LocalName] = tr.RemoteName
		} else {
			// For outbound listener,
			//   - incoming requests from local server are modifed to match remote server
			//   - outgoing responses to remote server are modified to match local server
			requestNameMapping[tr.LocalName] = tr.RemoteName
			responseNameMapping[tr.RemoteName] = tr.LocalName
		}
	}

	return &NamespaceNameTranslator{
		logger:              logger,
		requestNameMapping:  requestNameMapping,
		responseNameMapping: responseNameMapping,
	}
}

var _ grpc.UnaryServerInterceptor = (*NamespaceNameTranslator)(nil).Intercept
var _ grpc.StreamServerInterceptor = (*NamespaceNameTranslator)(nil).InterceptStream

func createNameTranslator(mapping map[string]string) matcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}

func (i *NamespaceNameTranslator) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if len(i.requestNameMapping) == 0 {
		return handler(ctx, req)
	}

	if strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) ||
		strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {

		methodName := api.MethodName(info.FullMethod)
		i.logger.Debug("intercepted request", tag.NewStringTag("method", methodName))

		// Translate namespace name in request.
		changed, trErr := visitNamespace(req, createNameTranslator(i.requestNameMapping))
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		if methodName == "GetWorkflowExecutionRawHistoryV2" {
			i.logger.Debug("unpacking GetWorkflowExecutionRawHistoryV2 response datablobs")
			cast, ok := resp.(*adminservice.GetWorkflowExecutionRawHistoryV2Response)
			if !ok {
				i.logger.Warn("failed to cast resp to GetWorkflowExecutionRawHistoryV2Response (skipping)")
			} else {
				cast.HistoryBatches = translateDataBlobs(i.logger, i.responseNameMapping, cast.HistoryBatches...)
			}
		}

		// Translate namespace name in response.
		changed, trErr = visitNamespace(resp, createNameTranslator(i.responseNameMapping))
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Response", resp)
		return resp, err
	} else {
		return handler(ctx, req)
	}
}

func (i *NamespaceNameTranslator) InterceptStream(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	i.logger.Debug("InterceptStream!",
		tag.NewAnyTag("method", info.FullMethod),
		tag.NewAnyTag("requestMap", i.requestNameMapping),
		tag.NewAnyTag("responseMap", i.responseNameMapping),
	)
	err := handler(srv, newWrappedStream(
		ss,
		i.logger,
		i.requestNameMapping,
		i.responseNameMapping,
	))
	if err != nil {
		i.logger.Error("RPC failed with error: %v", tag.Error(err))
	}
	return err
}

type wrappedStream struct {
	grpc.ServerStream
	logger              log.Logger
	requestNameMapping  map[string]string
	responseNameMapping map[string]string
}

// Inbound:
//
//	Recv from remote server
//	Send to local server
//
// Outbound:
//
//	Recv from local server
//	Send to remote server
func (w *wrappedStream) RecvMsg(m any) error {
	w.logger.Debug("Intercept RecvMsg", tag.NewAnyTag("message", m))

	changed, trErr := visitNamespace(m, createNameTranslator(w.requestNameMapping))
	logTranslateNamespaceResult(w.logger, changed, trErr, "RecvMsg", m)

	return w.ServerStream.RecvMsg(m)
}

func (w *wrappedStream) SendMsg(m any) error {
	w.logger.Debug("Intercept SendMsg", tag.NewStringTag("type", fmt.Sprintf("%T", m)), tag.NewAnyTag("message", m))

	cast, ok := m.(*adminservice.StreamWorkflowReplicationMessagesResponse)
	if ok && cast.Attributes != nil {
		attrs, ok := cast.Attributes.(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages)
		if ok {
			for _, task := range attrs.Messages.ReplicationTasks {
				w.logger.Debug("SendMsg: ReplicationTask", tag.NewAnyTag("task", cast))
				if task.Data != nil {
					task.Data = translateDataBlobs(w.logger, w.responseNameMapping, task.Data)[0]
				}
				//case *repication.ReplicationTask_NamespaceTaskAttributes:
				//case *repication.ReplicationTask_SyncShardStatusTaskAttributes:
				//case *repication.ReplicationTask_SyncActivityTaskAttributes:
				//case *repication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				//case *repication.ReplicationTask_TaskQueueUserDataAttributes:

				switch ta := task.Attributes.(type) {
				case *repication.ReplicationTask_HistoryTaskAttributes:
					if ta.HistoryTaskAttributes.Events != nil {
						ta.HistoryTaskAttributes.Events = translateDataBlobs(
							w.logger,
							w.responseNameMapping,
							ta.HistoryTaskAttributes.Events,
						)[0]
					}
					// Idk if I need to translate this but whatev
					if ta.HistoryTaskAttributes.NewRunEvents != nil {
						ta.HistoryTaskAttributes.NewRunEvents = translateDataBlobs(
							w.logger,
							w.responseNameMapping,
							ta.HistoryTaskAttributes.NewRunEvents,
						)[0]
					}
				default:
					w.logger.Warn("skip translating replication task attributes",
						tag.NewStringTag("type", fmt.Sprintf("%T", ta)))

				}
			}
		}
	}

	changed, trErr := visitNamespace(m, createNameTranslator(w.responseNameMapping))
	logTranslateNamespaceResult(w.logger, changed, trErr, "SendMsg", m)

	return w.ServerStream.SendMsg(m)
}

func newWrappedStream(
	s grpc.ServerStream,
	logger log.Logger,
	requestMapping map[string]string,
	responseMapping map[string]string,
) grpc.ServerStream {
	return &wrappedStream{
		ServerStream:        s,
		logger:              logger,
		requestNameMapping:  requestMapping,
		responseNameMapping: responseMapping,
	}
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

func translateDataBlobs(
	logger log.Logger,
	mapping map[string]string,
	blobs ...*common.DataBlob,
) []*common.DataBlob {
	s := serialization.NewSerializer()
	translator := createNameTranslator(mapping)

	var translated []*common.DataBlob
	for _, blob := range blobs {
		evt, err := s.DeserializeEvents(blob)
		if err != nil {
			logger.Warn("failed to deserialize history data blob (skipping)", tag.Error(err))
			translated = append(translated, blob)
			continue
		}

		changed, trErr := visitNamespace(evt, translator)
		logTranslateNamespaceResult(logger, changed, trErr, "DataBlob", evt)

		newBlob, err := s.SerializeEvents(evt, blob.EncodingType)
		if err != nil {
			logger.Error("failed to serialize history data blob (skipping)", tag.Error(err))
			translated = append(translated, blob)
		} else {
			translated = append(translated, newBlob)
		}
	}
	return translated
}
