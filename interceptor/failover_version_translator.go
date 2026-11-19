package interceptor

import (
	"errors"
	"fmt"
	"strings"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	servercodec "go.temporal.io/server/common/codec"
	"go.temporal.io/server/common/persistence/serialization"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/temporalio/s2s-proxy/failoverversion"
	"github.com/temporalio/s2s-proxy/metrics"
)

type versionTranslator interface {
	TranslateVersion(int64) (int64, error)
	TranslateFailoverVersionIncrement(int64) (int64, error)
	TranslateLane(int64) (int64, error)
	TranslateLaneInt32(int32) (int32, error)
	Direction() failoverversion.Direction
	MappingDescriptor() failoverversion.MappingDescriptor
}

type HistoryEventCodec interface {
	Decode(*commonpb.DataBlob) ([]*historypb.HistoryEvent, error)
	Encode(enumspb.EncodingType, []*historypb.HistoryEvent) (*commonpb.DataBlob, error)
}

type FailoverVersionTranslator struct {
	versions versionTranslator
	history  HistoryEventCodec
}

type FailoverVersionTranslatorOption func(*FailoverVersionTranslator)

func WithHistoryEventCodec(codec HistoryEventCodec) FailoverVersionTranslatorOption {
	return func(translator *FailoverVersionTranslator) {
		if codec != nil {
			translator.history = codec
		}
	}
}

func NewFailoverVersionTranslator(versions failoverversion.Translator, options ...FailoverVersionTranslatorOption) *FailoverVersionTranslator {
	translator := &FailoverVersionTranslator{versions: versions, history: productionHistoryEventCodec{}}
	for _, option := range options {
		option(translator)
	}
	return translator
}

func (t *FailoverVersionTranslator) Kind() string { return metrics.FailoverVersionTranslationKind }

func (t *FailoverVersionTranslator) Direction() failoverversion.Direction {
	return t.versions.Direction()
}

func (t *FailoverVersionTranslator) MappingDescriptor() failoverversion.MappingDescriptor {
	return t.versions.MappingDescriptor()
}

func (t *FailoverVersionTranslator) TranslateMessage(message proto.Message) (TranslationResult, error) {
	if message == nil || !message.ProtoReflect().IsValid() {
		return TranslationResult{Message: message}, nil
	}
	clone := proto.Clone(message)
	report := TranslationReport{}
	changed, err := t.walkMessage(clone.ProtoReflect(), &report)
	if err != nil {
		return TranslationResult{}, err
	}
	report.Changed = changed
	return TranslationResult{Message: clone, Report: report}, nil
}

func (t *FailoverVersionTranslator) TranslateError(err error) ErrorTranslationResult {
	if err == nil {
		return ErrorTranslationResult{}
	}
	st, ok := status.FromError(err)
	if !ok || len(st.Proto().Details) == 0 {
		return ErrorTranslationResult{Error: err}
	}
	// Clone the wire status directly so its code and message cannot change.
	statusProto := proto.Clone(st.Proto()).(*statuspb.Status)
	changed := false
	report := TranslationReport{}
	for i, detail := range statusProto.Details {
		message, decodeErr := anypb.UnmarshalNew(detail, proto.UnmarshalOptions{})
		if decodeErr != nil {
			// Truly unknown status-detail types are opaque and pass through. The
			// classified Temporal details are safety-critical and must decode.
			if !isClassifiedStatusDetail(detail.GetTypeUrl()) {
				continue
			}
			return ErrorTranslationResult{Error: failoverversion.NewTranslationError(
				failoverversion.ErrorStatusDetail,
				t.versions.Direction(),
				"grpc.status",
				"details",
				decodeErr,
			)}
		}
		result, translateErr := t.TranslateMessage(message)
		if translateErr != nil {
			return ErrorTranslationResult{Error: translateErr}
		}
		if !result.Report.Changed {
			continue
		}
		translated, encodeErr := anypb.New(result.Message)
		if encodeErr != nil {
			return ErrorTranslationResult{Error: failoverversion.NewTranslationError(
				failoverversion.ErrorStatusDetail,
				t.versions.Direction(),
				string(message.ProtoReflect().Descriptor().FullName()),
				"details",
				encodeErr,
			)}
		}
		statusProto.Details[i] = translated
		changed = true
		report.merge(result.Report)
		report.StatusDetails++
	}
	if !changed {
		return ErrorTranslationResult{Error: err}
	}
	report.Changed = true
	return ErrorTranslationResult{Error: status.FromProto(statusProto).Err(), Report: report}
}

func isClassifiedStatusDetail(typeURL string) bool {
	return strings.HasSuffix(typeURL, "/temporal.server.api.errordetails.v1.RetryReplicationFailure") ||
		strings.HasSuffix(typeURL, "/temporal.server.api.errordetails.v1.SyncStateFailure")
}

type scalarSemantic uint8

const (
	scalarNone scalarSemantic = iota
	scalarVersion
	scalarFVI
	scalarLane
	scalarLaneInt32
)

// FailoverVersionFieldKind is the reviewed semantic meaning of a protobuf
// field. The registry is also exposed to descriptor-audit tests so dependency
// upgrades cannot silently add or remove a classified path.
type FailoverVersionFieldKind string

const (
	FailoverVersionFieldVersion        FailoverVersionFieldKind = "version"
	FailoverVersionFieldFVI            FailoverVersionFieldKind = "fvi"
	FailoverVersionFieldLane           FailoverVersionFieldKind = "lane"
	FailoverVersionFieldClusterID      FailoverVersionFieldKind = "cluster_id"
	FailoverVersionFieldEncodedHistory FailoverVersionFieldKind = "encoded_history"
	FailoverVersionFieldExcluded       FailoverVersionFieldKind = "excluded"
)

type FailoverVersionFieldClassification struct {
	Kind   FailoverVersionFieldKind
	Reason string
}

var versionFields = map[protoreflect.FullName]struct{}{
	"temporal.api.history.v1.HistoryEvent.version":                                                    {},
	"temporal.api.history.v1.WorkflowExecutionStartedEventAttributes.parent_initiated_event_version":  {},
	"temporal.api.history.v1.WorkflowTaskFailedEventAttributes.fork_event_version":                    {},
	"temporal.server.api.history.v1.VersionHistoryItem.version":                                       {},
	"temporal.server.api.history.v1.StrippedHistoryEvent.version":                                     {},
	"temporal.server.api.workflow.v1.BaseExecutionInfo.lowest_common_ancestor_event_version":          {},
	"temporal.server.api.workflow.v1.ParentExecutionInfo.initiated_version":                           {},
	"temporal.server.api.persistence.v1.WorkflowExecutionInfo.workflow_task_version":                  {},
	"temporal.server.api.persistence.v1.WorkflowExecutionInfo.parent_initiated_version":               {},
	"temporal.server.api.persistence.v1.ActivityInfo.version":                                         {},
	"temporal.server.api.persistence.v1.ActivityInfo.start_version":                                   {},
	"temporal.server.api.persistence.v1.TimerInfo.version":                                            {},
	"temporal.server.api.persistence.v1.ChildExecutionInfo.version":                                   {},
	"temporal.server.api.persistence.v1.RequestCancelInfo.version":                                    {},
	"temporal.server.api.persistence.v1.SignalInfo.version":                                           {},
	"temporal.server.api.persistence.v1.ReplicationTaskInfo.version":                                  {},
	"temporal.server.api.persistence.v1.TransferTaskInfo.version":                                     {},
	"temporal.server.api.persistence.v1.VisibilityTaskInfo.version":                                   {},
	"temporal.server.api.persistence.v1.TimerTaskInfo.version":                                        {},
	"temporal.server.api.persistence.v1.ArchivalTaskInfo.version":                                     {},
	"temporal.server.api.replication.v1.SyncActivityTaskAttributes.version":                           {},
	"temporal.server.api.replication.v1.SyncActivityTaskAttributes.start_version":                     {},
	"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryV2Request.start_event_version": {},
	"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryV2Request.end_event_version":   {},
	"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryRequest.start_event_version":   {},
	"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryRequest.end_event_version":     {},
	"temporal.server.api.adminservice.v1.ResendReplicationTasksRequest.start_version":                 {},
	"temporal.server.api.adminservice.v1.ResendReplicationTasksRequest.end_version":                   {},
	"temporal.server.api.errordetails.v1.RetryReplicationFailure.start_event_version":                 {},
	"temporal.server.api.errordetails.v1.RetryReplicationFailure.end_event_version":                   {},
	"temporal.server.api.adminservice.v1.Task.version":                                                {},
	"temporal.server.api.archiver.v1.HistoryBlobHeader.first_failover_version":                        {},
	"temporal.server.api.archiver.v1.HistoryBlobHeader.last_failover_version":                         {},
}

var laneFields = map[protoreflect.FullName]struct{}{
	"temporal.server.api.clock.v1.VectorClock.cluster_id":        {},
	"temporal.server.api.clock.v1.HybridLogicalClock.cluster_id": {},
}

var laneInt32Fields = map[protoreflect.FullName]struct{}{
	"temporal.server.api.adminservice.v1.SyncWorkflowStateRequest.target_cluster_id":   {},
	"temporal.server.api.historyservice.v1.SyncWorkflowStateRequest.target_cluster_id": {},
}

var encodedHistoryFields = map[protoreflect.FullName]struct{}{
	"temporal.server.api.replication.v1.HistoryTaskAttributes.events":                              {},
	"temporal.server.api.replication.v1.HistoryTaskAttributes.new_run_events":                      {},
	"temporal.server.api.replication.v1.HistoryTaskAttributes.events_batches":                      {},
	"temporal.server.api.replication.v1.BackfillHistoryTaskAttributes.event_batches":               {},
	"temporal.server.api.replication.v1.NewRunInfo.event_batch":                                    {},
	"temporal.server.api.replication.v1.VersionedTransitionArtifact.event_batches":                 {},
	"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryV2Response.history_batches": {},
	"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryResponse.history_batches":   {},
	"temporal.api.workflowservice.v1.GetWorkflowExecutionHistoryResponse.raw_history":              {},
	"temporal.server.api.adminservice.v1.ImportWorkflowExecutionRequest.history_batches":           {},
	"temporal.server.api.adminservice.v1.ReapplyEventsRequest.events":                              {},
}

var excludedFailoverVersionFields = map[protoreflect.FullName]string{
	"temporal.server.api.replication.v1.NamespaceTaskAttributes.config_version":        "namespace configuration revision, not a failover version",
	"temporal.server.api.persistence.v1.NamespaceDetail.failover_notification_version": "namespace notification revision, not a failover version",
	"temporal.server.api.clock.v1.HybridLogicalClock.version":                          "hybrid logical clock revision, not a failover version",
}

// ClassifiedFailoverVersionFields returns a copy of the explicit field
// registry. Exact-name policies such as failover_version are applied by
// ClassifyFailoverVersionField and are intentionally not expanded here.
func ClassifiedFailoverVersionFields() map[protoreflect.FullName]FailoverVersionFieldClassification {
	classified := make(map[protoreflect.FullName]FailoverVersionFieldClassification,
		len(versionFields)+len(laneFields)+len(laneInt32Fields)+len(encodedHistoryFields)+len(excludedFailoverVersionFields))
	for field := range versionFields {
		classified[field] = FailoverVersionFieldClassification{Kind: FailoverVersionFieldVersion}
	}
	for field := range laneFields {
		classified[field] = FailoverVersionFieldClassification{Kind: FailoverVersionFieldLane}
	}
	for field := range laneInt32Fields {
		classified[field] = FailoverVersionFieldClassification{Kind: FailoverVersionFieldClusterID}
	}
	for field := range encodedHistoryFields {
		classified[field] = FailoverVersionFieldClassification{Kind: FailoverVersionFieldEncodedHistory}
	}
	for field, reason := range excludedFailoverVersionFields {
		classified[field] = FailoverVersionFieldClassification{Kind: FailoverVersionFieldExcluded, Reason: reason}
	}
	return classified
}

// ClassifyFailoverVersionField applies the reviewed explicit registry and the
// small set of exact-name policies used by Temporal for semantic versions.
func ClassifyFailoverVersionField(field protoreflect.FieldDescriptor) (FailoverVersionFieldClassification, bool) {
	fullName := field.FullName()
	if _, ok := versionFields[fullName]; ok {
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldVersion}, true
	}
	if _, ok := laneFields[fullName]; ok {
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldLane}, true
	}
	if _, ok := laneInt32Fields[fullName]; ok {
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldClusterID}, true
	}
	if _, ok := encodedHistoryFields[fullName]; ok {
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldEncodedHistory}, true
	}
	if reason, ok := excludedFailoverVersionFields[fullName]; ok {
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldExcluded, Reason: reason}, true
	}
	switch field.Name() {
	case "failover_version", "namespace_failover_version":
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldVersion}, true
	case "failover_version_increment":
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldFVI}, true
	case "initial_failover_version":
		return FailoverVersionFieldClassification{Kind: FailoverVersionFieldLane}, true
	default:
		return FailoverVersionFieldClassification{}, false
	}
}

func classifyScalar(field protoreflect.FieldDescriptor) scalarSemantic {
	classification, ok := ClassifyFailoverVersionField(field)
	if !ok {
		return scalarNone
	}
	switch classification.Kind {
	case FailoverVersionFieldVersion:
		return scalarVersion
	case FailoverVersionFieldFVI:
		return scalarFVI
	case FailoverVersionFieldLane:
		return scalarLane
	case FailoverVersionFieldClusterID:
		return scalarLaneInt32
	default:
		return scalarNone
	}
}

func (t *FailoverVersionTranslator) walkMessage(message protoreflect.Message, report *TranslationReport) (bool, error) {
	descriptor := message.Descriptor()
	messageName := descriptor.FullName()
	changed := false

	if messageName == "temporal.server.api.replication.v1.ReplicationTask" {
		dataField := descriptor.Fields().ByName("data")
		if dataField != nil && message.Has(dataField) {
			if blob, ok := message.Get(dataField).Message().Interface().(*commonpb.DataBlob); ok && len(blob.GetData()) != 0 {
				return false, failoverversion.NewTranslationError(
					failoverversion.ErrorUnsupported,
					t.versions.Direction(),
					string(messageName),
					"data",
					errors.New("opaque replication task payload"),
				)
			}
		}
	}

	fields := descriptor.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if !message.Has(field) && !field.IsList() && !field.IsMap() {
			continue
		}
		fieldChanged, err := t.walkField(message, field, report)
		if err != nil {
			fieldPath := string(field.Name())
			var translationErr *failoverversion.TranslationError
			if errors.As(err, &translationErr) && translationErr.FieldPath != "" {
				fieldPath += "." + translationErr.FieldPath
			}
			return changed, failoverversion.WithField(err, string(messageName), fieldPath)
		}
		changed = changed || fieldChanged
	}

	if changed && messageName == "temporal.server.api.persistence.v1.WorkflowMutableState" {
		checksum := fields.ByName("checksum")
		if checksum != nil && message.Has(checksum) {
			message.Clear(checksum)
			report.ChecksumsCleared++
		}
	}
	return changed, nil
}

func (t *FailoverVersionTranslator) walkField(message protoreflect.Message, field protoreflect.FieldDescriptor, report *TranslationReport) (bool, error) {
	if classification, ok := ClassifyFailoverVersionField(field); ok && classification.Kind == FailoverVersionFieldEncodedHistory {
		return t.translateHistoryField(message, field, report)
	}

	if semantic := classifyScalar(field); semantic != scalarNone {
		return t.translateScalarField(message, field, semantic, report)
	}

	if field.IsMap() {
		if field.MapValue().Kind() != protoreflect.MessageKind {
			return false, nil
		}
		changed := false
		var walkErr error
		message.Get(field).Map().Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
			itemChanged, err := t.walkMessage(value.Message(), report)
			changed = changed || itemChanged
			walkErr = err
			return err == nil
		})
		return changed, walkErr
	}
	if field.IsList() {
		if field.Kind() != protoreflect.MessageKind {
			return false, nil
		}
		list := message.Get(field).List()
		changed := false
		for i := 0; i < list.Len(); i++ {
			itemChanged, err := t.walkMessage(list.Get(i).Message(), report)
			changed = changed || itemChanged
			if err != nil {
				return changed, err
			}
		}
		return changed, nil
	}
	if field.Kind() == protoreflect.MessageKind && message.Has(field) {
		return t.walkMessage(message.Get(field).Message(), report)
	}
	return false, nil
}

func (t *FailoverVersionTranslator) translateScalarField(
	message protoreflect.Message,
	field protoreflect.FieldDescriptor,
	semantic scalarSemantic,
	report *TranslationReport,
) (bool, error) {
	value := message.Get(field)
	switch semantic {
	case scalarVersion:
		translated, err := t.versions.TranslateVersion(value.Int())
		if err != nil {
			return false, err
		}
		if translated == value.Int() {
			return false, nil
		}
		message.Set(field, protoreflect.ValueOfInt64(translated))
		report.VersionFields++
		return true, nil
	case scalarFVI:
		translated, err := t.versions.TranslateFailoverVersionIncrement(value.Int())
		if err != nil {
			return false, err
		}
		if translated == value.Int() {
			return false, nil
		}
		message.Set(field, protoreflect.ValueOfInt64(translated))
		report.FVIs++
		return true, nil
	case scalarLane:
		translated, err := t.versions.TranslateLane(value.Int())
		if err != nil {
			return false, err
		}
		if translated == value.Int() {
			return false, nil
		}
		message.Set(field, protoreflect.ValueOfInt64(translated))
		report.Lanes++
		return true, nil
	case scalarLaneInt32:
		translated, err := t.versions.TranslateLaneInt32(int32(value.Int()))
		if err != nil {
			return false, err
		}
		if int64(translated) == value.Int() {
			return false, nil
		}
		message.Set(field, protoreflect.ValueOfInt32(translated))
		report.ClusterIDs++
		return true, nil
	default:
		return false, nil
	}
}

func (t *FailoverVersionTranslator) translateHistoryField(message protoreflect.Message, field protoreflect.FieldDescriptor, report *TranslationReport) (bool, error) {
	if field.IsList() {
		list := message.Get(field).List()
		if list.Len() == 0 {
			return false, nil
		}
		staged := make([]*commonpb.DataBlob, list.Len())
		changed := false
		for i := 0; i < list.Len(); i++ {
			blob, ok := list.Get(i).Message().Interface().(*commonpb.DataBlob)
			if !ok {
				return false, fmt.Errorf("classified history field is not a DataBlob")
			}
			translated, itemChanged, err := t.translateHistoryBlob(blob, report)
			if err != nil {
				return false, err
			}
			staged[i] = translated
			changed = changed || itemChanged
		}
		if changed {
			list.Truncate(0)
			for _, blob := range staged {
				list.Append(protoreflect.ValueOfMessage(blob.ProtoReflect()))
			}
		}
		return changed, nil
	}
	blob, ok := message.Get(field).Message().Interface().(*commonpb.DataBlob)
	if !ok {
		return false, fmt.Errorf("classified history field is not a DataBlob")
	}
	translated, changed, err := t.translateHistoryBlob(blob, report)
	if err != nil || !changed {
		return changed, err
	}
	message.Set(field, protoreflect.ValueOfMessage(translated.ProtoReflect()))
	return true, nil
}

func (t *FailoverVersionTranslator) translateHistoryBlob(blob *commonpb.DataBlob, report *TranslationReport) (*commonpb.DataBlob, bool, error) {
	if blob == nil || len(blob.GetData()) == 0 {
		return blob, false, nil
	}
	events, err := t.history.Decode(blob)
	if err != nil {
		return blob, false, failoverversion.NewTranslationError(
			failoverversion.ErrorDecodeFailed,
			t.versions.Direction(),
			"temporal.api.common.v1.DataBlob",
			"data",
			err,
		)
	}
	changed := false
	for _, event := range events {
		itemChanged, translateErr := t.walkMessage(event.ProtoReflect(), report)
		changed = changed || itemChanged
		if translateErr != nil {
			return blob, false, translateErr
		}
	}
	if !changed {
		return blob, false, nil
	}
	translated, err := t.history.Encode(blob.GetEncodingType(), events)
	if err != nil {
		return blob, false, failoverversion.NewTranslationError(
			failoverversion.ErrorEncodeFailed,
			t.versions.Direction(),
			"temporal.api.common.v1.DataBlob",
			"data",
			err,
		)
	}
	report.HistoryBlobs++
	return translated, true, nil
}

type productionHistoryEventCodec struct{}

func (productionHistoryEventCodec) Decode(blob *commonpb.DataBlob) ([]*historypb.HistoryEvent, error) {
	events, err := serialization.NewSerializer().DeserializeEvents(blob)
	if err == nil {
		return events, nil
	}
	repaired, _, repairErr := tryRepairInvalidUTF8InBlob(blob)
	if repairErr != nil {
		return nil, err
	}
	return repaired, nil
}

func (productionHistoryEventCodec) Encode(encoding enumspb.EncodingType, events []*historypb.HistoryEvent) (*commonpb.DataBlob, error) {
	history := &historypb.History{Events: events}
	switch encoding {
	case enumspb.ENCODING_TYPE_PROTO3:
		data, err := proto.Marshal(history)
		if err != nil {
			return nil, err
		}
		return &commonpb.DataBlob{EncodingType: encoding, Data: data}, nil
	case enumspb.ENCODING_TYPE_JSON:
		data, err := servercodec.NewJSONPBEncoder().Encode(history)
		if err != nil {
			return nil, err
		}
		return &commonpb.DataBlob{EncodingType: encoding, Data: data}, nil
	default:
		return nil, fmt.Errorf("unsupported history encoding")
	}
}
