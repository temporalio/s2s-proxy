package interceptor

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/keilerkonzept/visit"
	"go.temporal.io/api/common/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/persistence/serialization"
)

const (
	namespaceTaskAttributesFieldName = "NamespaceTaskAttributes"
	historyTaskAttributesFieldName   = "HistoryTaskAttributes"
	historyBatchesFieldName          = "HistoryBatches" // in GetWorkflowExecutionRawHistoryV2
)

var (
	namespaceFieldNames = []string{
		"Namespace",
		"WorkflowNamespace", // PollActivityTaskQueueResponse
	}
)

// matcher returns 2 values:
//  1. new name. If there is no change, new name equals to input name
//  2. whether or not the input name matches the defined rule(s).
type matcher func(name string) (string, bool)

// visitNamespace uses reflection to recursively visit all fields
// in the given object. When it finds namespace string fields, it invokes
// the provided match function.
func visitNamespace(obj any, match matcher) (bool, error) {
	var matched bool

	// The visitor function can return Skip, Stop, or Continue to control recursion.
	err := visit.Values(obj, func(vwp visit.ValueWithParent) (visit.Action, error) {
		// Grab name of this struct field from the parent.
		if vwp.Parent == nil || vwp.Parent.Kind() != reflect.Struct {
			return visit.Continue, nil
		}
		fieldType := vwp.Parent.Type().Field(int(vwp.Index.Int()))
		if !fieldType.IsExported() {
			// Ignore unexported fields, particularly private gRPC message fields.
			return visit.Skip, nil
		}

		if fieldType.Name == namespaceTaskAttributesFieldName {
			// Handle NamespaceTaskAttributes.Info.Name in replication task attributes
			attrs, ok := vwp.Interface().(*replicationspb.NamespaceTaskAttributes)
			if !ok {
				return visit.Stop, fmt.Errorf("failed to cast *NamespaceTaskAttributes")
			}
			if attrs == nil || attrs.Info == nil {
				return visit.Continue, nil
			}

			newName, ok := match(attrs.Info.Name)
			if !ok {
				return visit.Continue, nil
			}
			if attrs.Info.Name != newName {
				attrs.Info.Name = newName
			}
			matched = matched || ok
		} else if fieldType.Name == historyTaskAttributesFieldName {
			// Handle HistoryTaskAttributes.Events in replication task attributes
			attrs, ok := vwp.Interface().(*replicationspb.HistoryTaskAttributes)
			if !ok {
				return visit.Stop, fmt.Errorf("failed to cast *HistoryTaskAttributes")
			}
			if attrs == nil || attrs.Events == nil {
				return visit.Continue, nil
			}

			var changed bool
			var err error
			attrs.Events, changed, err = translateDataBlob(attrs.Events, match)
			if err != nil {
				return visit.Stop, fmt.Errorf("error translating HistoryTaskAttributes.Events: %w", err)
			}
			matched = matched || changed

			attrs.NewRunEvents, changed, err = translateDataBlob(attrs.NewRunEvents, match)
			if err != nil {
				return visit.Stop, fmt.Errorf("error translating HistoryTaskAttributes.NewRunEvents: %w", err)
			}

			matched = matched || changed
		} else if fieldType.Name == historyBatchesFieldName {
			evt, ok := vwp.Interface().([]*common.DataBlob)
			if !ok {
				return visit.Stop, fmt.Errorf("failed to cast HistoryBatches")
			}

			var newEvt []*common.DataBlob
			anyChanged := false
			for _, e := range evt {
				tr, changed, err := translateDataBlob(e, match)
				if err != nil {
					return visit.Stop, err
				}
				anyChanged = anyChanged || changed
				newEvt = append(newEvt, tr)
			}

			if anyChanged {
				if err := visit.Assign(vwp, reflect.ValueOf(newEvt)); err != nil {
					return visit.Stop, err
				}
			}
			matched = matched || anyChanged
		} else if vwp.Kind() == reflect.String && slices.Contains(namespaceFieldNames, fieldType.Name) {
			newName, ok := match(vwp.String())
			if !ok {
				return visit.Continue, nil
			}

			if vwp.String() != newName {
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

func translateDataBlob(blob *common.DataBlob, match matcher) (*common.DataBlob, bool, error) {
	if blob == nil {
		return blob, false, nil
	}

	s := serialization.NewSerializer()
	evt, err := s.DeserializeEvents(blob)
	if err != nil {
		return blob, false, err
	}

	changed, err := visitNamespace(evt, match)
	if err != nil {
		return blob, false, err
	}

	newBlob, err := s.SerializeEvents(evt, blob.EncodingType)
	if err != nil {
		return blob, false, err
	}
	return newBlob, changed, nil
}
