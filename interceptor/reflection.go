package interceptor

import (
	"fmt"
	"reflect"

	"github.com/keilerkonzept/visit"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/persistence/serialization"

	s2scommon "github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/metrics"
	common122 "github.com/temporalio/s2s-proxy/proto/1_22/api/common/v1"
	enums122 "github.com/temporalio/s2s-proxy/proto/1_22/api/enums/v1"
	history122 "github.com/temporalio/s2s-proxy/proto/1_22/api/history/v1"
	serialization122 "github.com/temporalio/s2s-proxy/proto/1_22/server/common/persistence/serialization"
	"github.com/temporalio/s2s-proxy/proto/compat"
)

var (
	serializer     = serialization.NewSerializer()
	gogoSerializer = serialization122.NewSerializer()

	namespaceFieldNames = map[string]bool{
		"Namespace":               true,
		"WorkflowNamespace":       true, // PollActivityTaskQueueResponse
		"ParentWorkflowNamespace": true, // WorkflowExecutionStartedEventAttributes
	}

	dataBlobFieldNames = map[string]bool{
		"Events":         true, // HistoryTaskAttributes
		"NewRunEvents":   true, // HistoryTaskAttributes
		"EventBatch":     true, // NewRunInfo type
		"EventBatches":   true, // BackfillHistoryTaskAttributes, VersionedTransitionArtifact
		"EventsBatches":  true, // HistoryTaskAttributes
		"HistoryBatches": true, // GetWorkflowExecutionRawHistoryV2
	}

	searchAttributeFieldNames = map[string]bool{
		// common.SearchAttributes
		// - WorkflowExecutionStartedEventAttributes
		// - WorkflowExecutionContinuedAsNewEventAttributes
		// - UpsertWorkflowSearchAttributesEventAttributes
		// - StartChildWorkflowExecutionInitiatedEventAttributes
		// map[string]*Payload:
		// - WorkflowExecutionInfo
		"SearchAttributes": true,
	}

	namespaceTranslationSkippableHistoryEvents = map[enums.EventType]struct{}{
		//enums.EVENT_TYPE_UNSPECIFIED:                       {},
		// Workflow Execution Started has a namespace field.
		//enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:        {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:        {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:           {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:        {},
		enums.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED:             {},
		enums.EVENT_TYPE_WORKFLOW_TASK_STARTED:               {},
		enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED:             {},
		enums.EVENT_TYPE_WORKFLOW_TASK_TIMED_OUT:             {},
		enums.EVENT_TYPE_WORKFLOW_TASK_FAILED:                {},
		enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:             {},
		enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:               {},
		enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:             {},
		enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:                {},
		enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:             {},
		enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED:      {},
		enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED:              {},
		enums.EVENT_TYPE_TIMER_STARTED:                       {},
		enums.EVENT_TYPE_TIMER_FIRED:                         {},
		enums.EVENT_TYPE_TIMER_CANCELED:                      {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCEL_REQUESTED: {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:         {},

		// Not these. "External" events have namespace field.
		//enums.EVENT_TYPE_REQUEST_CANCEL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED: {},
		//enums.EVENT_TYPE_REQUEST_CANCEL_EXTERNAL_WORKFLOW_EXECUTION_FAILED:    {},
		//enums.EVENT_TYPE_EXTERNAL_WORKFLOW_EXECUTION_CANCEL_REQUESTED:         {},

		enums.EVENT_TYPE_MARKER_RECORDED:                     {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED:         {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:       {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_CONTINUED_AS_NEW: {},

		// Not these. "Child" events have namespace field.
		//enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED: {},
		//enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_FAILED:    {},
		//enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_STARTED:         {},
		//enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_COMPLETED:       {},
		//enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_FAILED:          {},
		//enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_CANCELED:        {},
		//enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TIMED_OUT:       {},
		//enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TERMINATED:      {},

		// Not these. "External" events have namespace field.
		//enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED: {},
		//enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_FAILED:    {},
		//enums.EVENT_TYPE_EXTERNAL_WORKFLOW_EXECUTION_SIGNALED:         {},

		enums.EVENT_TYPE_UPSERT_WORKFLOW_SEARCH_ATTRIBUTES:       {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ADMITTED:      {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ACCEPTED:      {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_REJECTED:      {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_COMPLETED:     {},
		enums.EVENT_TYPE_WORKFLOW_PROPERTIES_MODIFIED_EXTERNALLY: {},
		enums.EVENT_TYPE_ACTIVITY_PROPERTIES_MODIFIED_EXTERNALLY: {},
		enums.EVENT_TYPE_WORKFLOW_PROPERTIES_MODIFIED:            {},
		enums.EVENT_TYPE_NEXUS_OPERATION_SCHEDULED:               {},
		enums.EVENT_TYPE_NEXUS_OPERATION_STARTED:                 {},
		enums.EVENT_TYPE_NEXUS_OPERATION_COMPLETED:               {},
		enums.EVENT_TYPE_NEXUS_OPERATION_FAILED:                  {},
		enums.EVENT_TYPE_NEXUS_OPERATION_CANCELED:                {},
		enums.EVENT_TYPE_NEXUS_OPERATION_TIMED_OUT:               {},
		enums.EVENT_TYPE_NEXUS_OPERATION_CANCEL_REQUESTED:        {},
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_OPTIONS_UPDATED:      {},
	}
)

// stringMatcher returns 2 values:
//  1. new name. If there is no change, new name equals to input name
//  2. whether or not the input name matches the defined rule(s).
type stringMatcher func(name string) (string, bool)

// visitor visits each field in obj matching the matcher.
// It returns whether anything was matched and any error it encountered.
type visitor func(logger log.Logger, obj any, match stringMatcher) (bool, error)

// visitNamespace uses reflection to recursively visit all fields
// in the given object. When it finds namespace string fields, it invokes
// the provided match function.
func visitNamespace(logger log.Logger, obj any, match stringMatcher) (bool, error) {
	if isSkippableForNamespaceTranslation(obj) {
		return false, nil
	}

	var matched bool

	// The visitor function can return Skip, Stop, or Continue to control recursion.
	err := visit.Values(obj, func(vwp visit.ValueWithParent) (visit.Action, error) {
		if vwp.Kind() == reflect.Ptr && vwp.IsNil() {
			return visit.Skip, nil
		}

		// Grab name of this struct field from the parent.
		fieldType, action := getParentFieldType(vwp)
		if action != "" {
			return action, nil
		}

		if info, ok := vwp.Interface().(*namespace.NamespaceInfo); ok && info != nil {
			// Handle NamespaceInfo.Name in any message.
			newName, ok := match(info.Name)
			if !ok {
				return visit.Continue, nil
			}
			if info.Name != newName {
				info.Name = newName
			}
			matched = matched || ok
		} else if hist, ok := vwp.Interface().(*history.History); ok && hist != nil {
			for _, evt := range hist.GetEvents() {
				// Do the recursive call here so that we check `isSkippableForNamespaceTranslation`.
				m, err := visitNamespace(logger, evt, match)
				matched = matched || m
				if err != nil {
					return visit.Stop, err
				}
			}
			return visit.Continue, nil
		} else if dataBlobFieldNames[fieldType.Name] {
			changed, err := visitDataBlobs(logger, vwp, match, visitNamespace)
			matched = matched || changed
			if err != nil {
				return visit.Stop, err
			}
		} else if namespaceFieldNames[fieldType.Name] {
			name, ok := vwp.Interface().(string)
			if !ok {
				return visit.Continue, nil
			}
			newName, ok := match(name)
			if !ok {
				return visit.Continue, nil
			}
			if name != newName {
				if err := visit.Assign(vwp, reflect.ValueOf(newName)); err != nil {
					return visit.Stop, err
				}
			}
			matched = matched || ok
		}

		return visit.Continue, nil
	})
	return matched, err
}

// visitSearchAttributes uses reflection to recursively visit all fields
// in the given object. When it finds namespace string fields, it invokes
// the provided match function.
func visitSearchAttributes(logger log.Logger, obj any, match stringMatcher) (bool, error) {
	var matched bool

	// The visitor function can return Skip, Stop, or Continue to control recursion.
	err := visit.Values(obj, func(vwp visit.ValueWithParent) (visit.Action, error) {
		if vwp.Kind() == reflect.Ptr && vwp.IsNil() {
			return visit.Skip, nil
		}

		// Grab name of this struct field from the parent.
		fieldType, action := getParentFieldType(vwp)
		if action != "" {
			return action, nil
		}
		if dataBlobFieldNames[fieldType.Name] {
			changed, err := visitDataBlobs(logger, vwp, match, visitSearchAttributes)
			matched = matched || changed
			if err != nil {
				return visit.Stop, err
			}
		} else if searchAttributeFieldNames[fieldType.Name] {
			// This could be *common.SearchAttributes, or it could be map[string]*common.Payload (indexed fields)
			var changed bool
			switch attrs := vwp.Interface().(type) {
			case *common.SearchAttributes:
				attrs.IndexedFields, changed = translateIndexedFields(attrs.IndexedFields, match)
			case map[string]*common.Payload:
				attrs, changed = translateIndexedFields(attrs, match)
				if changed {
					if err := visit.Assign(vwp, reflect.ValueOf(attrs)); err != nil {
						return visit.Stop, err
					}
				}
			default:
				return visit.Stop, fmt.Errorf("unhandled search attribute type: %T", attrs)
			}
			matched = matched || changed

			// No need to descend into this type further.
			return visit.Continue, nil
		}

		return visit.Continue, nil
	})
	return matched, err
}

func translateIndexedFields(fields map[string]*common.Payload, match stringMatcher) (map[string]*common.Payload, bool) {
	if fields == nil {
		return fields, false
	}

	var anyMatched bool
	newIndexed := make(map[string]*common.Payload, len(fields))
	for key, value := range fields {
		newKey, matched := match(key)
		anyMatched = anyMatched || matched
		if matched && key != newKey {
			newIndexed[newKey] = value
		} else {
			newIndexed[key] = value
		}
	}
	return newIndexed, anyMatched
}

func getParentFieldType(vwp visit.ValueWithParent) (result reflect.StructField, action visit.Action) {
	if vwp.Parent == nil || vwp.Parent.Kind() != reflect.Struct {
		return result, visit.Continue
	}
	fieldType := vwp.Parent.Type().Field(int(vwp.Index.Int()))
	if !fieldType.IsExported() {
		return result, visit.Skip
	}
	return fieldType, action
}

func visitDataBlobs(logger log.Logger, vwp visit.ValueWithParent, match stringMatcher, visitor visitor) (bool, error) {
	switch evt := vwp.Interface().(type) {
	case []*common.DataBlob:
		newEvts, matched, changed, err := translateDataBlobs(logger, match, visitor, evt...)
		if err != nil {
			return matched, err
		}
		if matched || changed {
			if err := visit.Assign(vwp, reflect.ValueOf(newEvts)); err != nil {
				return matched, err
			}
		}
		return matched, nil
	case *common.DataBlob:
		newEvt, matched, changed, err := translateOneDataBlob(logger, match, visitor, evt)
		if err != nil {
			return matched, err
		}
		if matched || changed {
			if err := visit.Assign(vwp, reflect.ValueOf(newEvt)); err != nil {
				return matched, err
			}
		}
		return matched, nil
	default:
		return false, nil
	}
}

func translateDataBlobs(logger log.Logger, match stringMatcher, visitor visitor, blobs ...*common.DataBlob) (result []*common.DataBlob, anyMatched, anyChanged bool, retErr error) {
	for i, blob := range blobs {
		newBlob, matched, changed, err := translateOneDataBlob(logger, match, visitor, blob)
		anyChanged = anyChanged || changed
		anyMatched = anyMatched || matched
		if err != nil {
			return blobs, anyMatched, anyChanged, err
		}
		blobs[i] = newBlob
	}
	return blobs, anyMatched, anyChanged, nil
}

func translateOneDataBlob(logger log.Logger, match stringMatcher, visitor visitor, blob *common.DataBlob) (result *common.DataBlob, matched, changed bool, retErr error) {
	if blob == nil || len(blob.Data) == 0 {
		return blob, matched, changed, nil
	}

	events, err := serializer.DeserializeEvents(blob)
	if err != nil {
		if !s2scommon.IsInvalidUTF8Error(err) {
			return blob, matched, changed, err
		}

		// A change due to repairing invalid UTF8 does not count as a "match".
		// For example, the access control visitor only wants to match if
		// a request is allowed or not.
		repairedEvents, c, err := tryRepairInvalidUTF8InBlob(blob)
		changed = changed || c
		if err != nil {
			logger.Error("failed to repair invalid utf-8 in history event blob", tag.Error(err))
			metrics.TranslationErrors.WithLabelValues(metrics.UTF8RepairTranslationKind, metrics.HistoryBlobMessageType).Inc()
			return blob, matched, changed, err
		} else if changed {
			logger.Debug("repaired invalid utf-8 in history event blob")
			metrics.TranslationCount.WithLabelValues(metrics.UTF8RepairTranslationKind, metrics.HistoryBlobMessageType).Inc()
			events = repairedEvents
		}
	}

	m, err := visitor(logger, events, match)
	matched = matched || m
	if err != nil {
		return blob, matched, changed, err
	}
	if matched || changed {
		blob, err = serializer.SerializeEvents(events, blob.EncodingType)
	}
	return blob, matched, changed, err
}

// tryRepairInvalidUTF8InBlob attempts to deserialize the blob as history events using old gogo-based protos.
// It returns the history events, which may be nil if (de)serializations fail, and a bool and error
// indicating if invalid UTF8 was repaired and whether there was any error.
func tryRepairInvalidUTF8InBlob(blob *common.DataBlob) ([]*history.HistoryEvent, bool, error) {
	// If we encountered a utf-8 error, try to repair it.
	encodingType122 := enums122.EncodingType(blob.EncodingType.Number())
	events122, err := gogoSerializer.DeserializeEvents(&common122.DataBlob{
		EncodingType: encodingType122,
		Data:         blob.Data,
	})
	if err != nil {
		return nil, false, err
	}

	changed, err := validateAndRepairHistoryEvents(events122)
	if err != nil || !changed {
		return nil, changed, err
	}

	// To avoid a bunch of type conversions, reserialize and deserialize with the new version.
	repairedEvents, err := gogoSerializer.SerializeEvents(events122, encodingType122)
	if err != nil {
		return nil, changed, err
	}
	events, err := serializer.DeserializeEvents(&common.DataBlob{
		EncodingType: blob.EncodingType,
		Data:         repairedEvents.Data,
	})
	return events, changed, err
}

func validateAndRepairHistoryEvents(events []*history122.HistoryEvent) (bool, error) {
	var changed bool
	for _, event := range events {
		c, err := compat.RepairInvalidUTF8(event)
		changed = changed || c
		if err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func isSkippableForNamespaceTranslation(vAny any) (result bool) {
	switch v := vAny.(type) {
	case *workflowservice.ListWorkflowExecutionsResponse:
		return true
	case []*history.HistoryEvent:
		for _, evt := range v {
			// If this namespace field is set, do not skip translation.
			for _, l := range evt.Links {
				if len(l.GetWorkflowEvent().GetNamespace()) > 0 {
					return false
				}
			}
			// Events with a namespace field should not skip translation.
			_, skippable := namespaceTranslationSkippableHistoryEvents[evt.GetEventType()]
			if !skippable {
				return false
			}
		}
		// Only skippable if all events in the list are skippable
		return true
	case *history.HistoryEvent:
		// If this namespace field is set, do not skip translation.
		for _, l := range v.Links {
			if len(l.GetWorkflowEvent().GetNamespace()) > 0 {
				return false
			}
		}
		// Events with a namespace field should not skip translation.
		_, skippable := namespaceTranslationSkippableHistoryEvents[v.GetEventType()]
		return skippable
	}

	return false
}
