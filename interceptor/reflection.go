package interceptor

import (
	"fmt"
	"reflect"

	"github.com/keilerkonzept/visit"
	"go.temporal.io/api/common/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
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
		"ClusterName":       true, // DescribeCluster, ListClusters, ReplicationTaska, GetNamespace (Clusters)
		"SourceCluster":     true, // HistoryDLQKey
		"TargetCluster":     true, // HistoryDLQKey
		"ActiveClusterName": true, // GetNamespace
	}
)

// matcher returns 2 values:
//  1. new name. If there is no change, new name equals to input name
//  2. whether or not the input name matches the defined rule(s).
type matcher func(name string) (string, bool)

type visitorFn func(obj any, match matcher) (bool, error)

type visitor struct {
	match            matcher
	extraVisitor     func(obj any, match matcher) (bool, visit.Action, error)
	fieldNameMatcher func(string) bool
	matched          bool
}

func (v *visitor) Visit(obj any) (matched bool, err error) {
	// The visitor function can return Skip, Stop, or Continue to control recursion.
	err = visit.Values(obj, v.visit)
	return v.matched, err
}

func (v *visitor) visitorFn(obj any, _ matcher) (matched bool, err error) {
	// The visitor function can return Skip, Stop, or Continue to control recursion.
	err = visit.Values(obj, v.visit)
	return v.matched, err
}

func (v *visitor) visit(vwp visit.ValueWithParent) (visit.Action, error) {
	// Grab name of this struct field from the parent.
	if vwp.Parent == nil || vwp.Parent.Kind() != reflect.Struct {
		return visit.Continue, nil
	}
	fieldType := vwp.Parent.Type().Field(int(vwp.Index.Int()))
	if !fieldType.IsExported() {
		// Ignore unexported fields, particularly private gRPC message fields.
		return visit.Skip, nil
	}

	if v.extraVisitor != nil {
		changed, action, err := v.extraVisitor(vwp.Interface(), v.match)
		if err != nil {
			return visit.Stop, err
		} else if changed {
			v.matched = true
		}
		return action, nil
	}

	if dataBlobFieldNames[fieldType.Name] {
		changed, err := visitDataBlobs(vwp, v.match, v.visitorFn)
		if err != nil {
			return visit.Stop, err
		}
		v.matched = v.matched || changed
	} else if v.fieldNameMatcher != nil && v.fieldNameMatcher(fieldType.Name) {
		changed, err := visitStringField(vwp, v.match)
		if err != nil {
			return visit.Stop, err
		}
		v.matched = v.matched || changed
	}

	return visit.Continue, nil
}

// visitNamespace uses reflection to recursively visit all fields
// in the given object. When it finds namespace string fields, it invokes
// the provided match function.
func visitNamespace(obj any, match matcher) (bool, error) {
	v := &visitor{
		match:            match,
		extraVisitor:     visitNamespaceInfo,
		fieldNameMatcher: isNamespaceFieldName,
	}
	return v.Visit(obj)
}

func visitNamespaceInfo(obj any, match matcher) (bool, visit.Action, error) {
	task, ok := obj.(*replicationspb.ReplicationTask)
	if !ok {
		return false, visit.Continue, nil
	}
	attrs, ok := task.Attributes.(*replicationspb.ReplicationTask_NamespaceTaskAttributes)
	if !ok {
		return false, visit.Continue, nil
	}

	// Handle NamespaceInfo.Name in any message.
	info := attrs.NamespaceTaskAttributes.Info

	// We can skip processing the rest of this task.
	newName, ok := match(info.Name)
	if !ok || info.Name == newName {
		return false, visit.Skip, nil
	}
	info.Name = newName
	return true, visit.Skip, nil
}

func isNamespaceFieldName(fieldName string) bool {
	return namespaceFieldNames[fieldName]
}

// visitClusterName uses reflection to recursively visit all fields
// in the given object. When it finds cluster name string fields, it invokes
// the provided match function.
func visitClusterName(obj any, match matcher) (bool, error) {
	v := &visitor{
		match:            match,
		fieldNameMatcher: isClusterNameFieldName,
	}
	return v.Visit(obj)
}

func isClusterNameFieldName(fieldName string) bool {
	return clusterNameFields[fieldName]
}

func visitDataBlobs(vwp visit.ValueWithParent, match matcher, visitor visitorFn) (bool, error) {
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

func translateOneDataBlob(match matcher, visitor visitorFn, blob *common.DataBlob) (*common.DataBlob, bool, error) {
	if blob == nil || len(blob.Data) == 0 {
		return blob, false, nil
	}
	blobs, changed, err := translateDataBlobs(match, visitor, blob)
	if err != nil {
		return nil, false, err
	}
	if len(blobs) != 1 {
		return nil, false, fmt.Errorf("failed to translate single data blob")
	}
	return blobs[0], changed, err
}

func translateDataBlobs(match matcher, visitor visitorFn, blobs ...*common.DataBlob) ([]*common.DataBlob, bool, error) {
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

		changed, err := visitor(evt, match)
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
