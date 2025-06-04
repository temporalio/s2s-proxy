package interceptor

import (
	"fmt"
	"reflect"

	"github.com/keilerkonzept/visit"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/server/common/persistence/serialization"
)

var (
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

	clusterNameFields = map[string]bool{
		"ClusterName":       true, // DescribeCluster, ListClusters, ReplicationTasks, GetNamespace (Clusters)
		"SourceCluster":     true, // HistoryDLQKey
		"TargetCluster":     true, // HistoryDLQKey
		"ActiveClusterName": true, // GetNamespace
	}
)

// matcher returns 2 values:
//  1. new name. If there is no change, new name equals to input name
//  2. whether or not the input name matches the defined rule(s).
type matcher func(name string) (string, bool)

// visitor visits each field in obj matching the matcher.
// It returns whether anything was matched and any error it encountered.
type visitor func(obj any, match matcher) (bool, error)

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
		} else if dataBlobFieldNames[fieldType.Name] {
			changed, err := visitDataBlobs(vwp, match, visitNamespace)
			if err != nil {
				return visit.Stop, err
			}
			matched = matched || changed
			return visit.Continue, nil
		} else if namespaceFieldNames[fieldType.Name] {
			changed, err := visitStringField(vwp, match)
			if err != nil {
				return visit.Stop, err
			}
			matched = matched || changed
			return visit.Continue, nil
		}

		return visit.Continue, nil
	})
	return matched, err
}

// visitClusterName uses reflection to recursively visit all fields
// in the given object. When it finds matching string fields, it invokes
// the provided match function.
func visitClusterName(obj any, match matcher) (bool, error) {
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

		if dataBlobFieldNames[fieldType.Name] {
			changed, err := visitDataBlobs(vwp, match, visitClusterName)
			if err != nil {
				return visit.Stop, err
			}
			matched = matched || changed
			return visit.Continue, nil
		} else if clusterNameFields[fieldType.Name] {
			changed, err := visitStringField(vwp, match)
			if err != nil {
				return visit.Stop, err
			}
			matched = matched || changed
			return visit.Continue, nil
		}

		return visit.Continue, nil
	})
	return matched, err
}

func visitDataBlobs(vwp visit.ValueWithParent, match matcher, visitor visitor) (bool, error) {
	switch evt := vwp.Interface().(type) {
	case []*common.DataBlob:
		newEvts, changed, err := translateDataBlobs(match, visitor, evt...)
		if err != nil {
			return changed, err
		}
		if changed {
			if err := visit.Assign(vwp, reflect.ValueOf(newEvts)); err != nil {
				return changed, err
			}
		}
		return changed, nil
	case *common.DataBlob:
		newEvt, changed, err := translateOneDataBlob(match, visitor, evt)
		if err != nil {
			return changed, err
		}
		if changed {
			if err := visit.Assign(vwp, reflect.ValueOf(newEvt)); err != nil {
				return changed, err
			}
		}
		return changed, nil
	default:
		return false, nil
	}
}

func visitStringField(vwp visit.ValueWithParent, match matcher) (bool, error) {
	name, ok := vwp.Interface().(string)
	if !ok {
		return false, nil
	}
	newName, ok := match(name)
	if !ok || name == newName {
		return false, nil
	}
	if err := visit.Assign(vwp, reflect.ValueOf(newName)); err != nil {
		return false, err
	}
	return true, nil
}

func translateOneDataBlob(match matcher, visit visitor, blob *common.DataBlob) (*common.DataBlob, bool, error) {
	if blob == nil || len(blob.Data) == 0 {
		return blob, false, nil
	}
	blobs, changed, err := translateDataBlobs(match, visit, blob)
	if err != nil {
		return nil, false, err
	}
	if len(blobs) != 1 {
		return nil, false, fmt.Errorf("failed to translate single data blob")
	}
	return blobs[0], changed, err
}

func translateDataBlobs(match matcher, visit visitor, blobs ...*common.DataBlob) ([]*common.DataBlob, bool, error) {
	if len(blobs) == 0 {
		return blobs, false, nil
	}

	s := serialization.NewSerializer()

	var anyChanged bool
	for i, blob := range blobs {
		evt, err := s.DeserializeEvents(blob)
		if err != nil {
			return blobs, anyChanged, err
		}

		changed, err := visit(evt, match)
		if err != nil {
			return blobs, anyChanged, err
		}
		anyChanged = anyChanged || changed

		newBlob, err := s.SerializeEvents(evt, blob.EncodingType)
		if err != nil {
			return blobs, anyChanged, err
		}
		blobs[i] = newBlob
	}

	return blobs, anyChanged, nil
}
