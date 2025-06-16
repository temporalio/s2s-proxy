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

	searchAttributeFieldNames = map[string]bool{
		"SearchAttributes": true, // UpsertWorkflowSearchAttributesEventAttributes, WorkflowExecutionStartedEventAttributes
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
		} else if dataBlobFieldNames[fieldType.Name] {
			changed, err := visitDataBlobs(vwp, match, visitNamespace)
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
func visitSearchAttributes(obj any, match matcher) (bool, error) {
	var matched bool

	// The visitor function can return Skip, Stop, or Continue to control recursion.
	err := visit.Values(obj, func(vwp visit.ValueWithParent) (visit.Action, error) {
		// Grab name of this struct field from the parent.
		fieldType, action := getParentFieldType(vwp)
		if action != "" {
			return action, nil
		}

		if dataBlobFieldNames[fieldType.Name] {
			changed, err := visitDataBlobs(vwp, match, visitSearchAttributes)
			matched = matched || changed
			if err != nil {
				return visit.Stop, err
			}
		} else if searchAttributeFieldNames[fieldType.Name] {
			fmt.Printf("match SA field %v\n", fieldType.Name)
			// This could be *common.SearchAttributes, or it could be map[string]*common.Payload (indexed fields)
			var changed bool
			fmt.Printf("before %+v\n", vwp.Interface())
			switch attrs := vwp.Interface().(type) {
			case *common.SearchAttributes:
				fmt.Println("match common.SearchAttributes")
				attrs.IndexedFields, changed = translateIndexedFields(attrs.IndexedFields, match)
			case map[string]*common.Payload:
				fmt.Println("match map[string]common.Payload")
				attrs, changed = translateIndexedFields(attrs, match)
				if changed {
					visit.Assign(vwp, reflect.ValueOf(attrs))
				}
			}
			fmt.Printf("changed=%v\n", changed)
			fmt.Printf("after %+v\n", vwp.Interface())
			matched = matched || changed

			// No need to descend into this type further.
			return visit.Continue, nil
		}

		return visit.Continue, nil
	})
	return matched, err
}

func translateIndexedFields(fields map[string]*common.Payload, match matcher) (map[string]*common.Payload, bool) {
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

func visitDataBlobs(vwp visit.ValueWithParent, match matcher, visitor visitor) (bool, error) {
	switch evt := vwp.Interface().(type) {
	case []*common.DataBlob:
		newEvts, matched, err := translateDataBlobs(match, visitor, evt...)
		if err != nil {
			return matched, err
		}
		if matched {
			if err := visit.Assign(vwp, reflect.ValueOf(newEvts)); err != nil {
				return matched, err
			}
		}
		return matched, nil
	case *common.DataBlob:
		newEvt, matched, err := translateOneDataBlob(match, visitor, evt)
		if err != nil {
			return matched, err
		}
		if matched {
			if err := visit.Assign(vwp, reflect.ValueOf(newEvt)); err != nil {
				return matched, err
			}
		}
		return matched, nil
	default:
		return false, nil
	}
}

func translateOneDataBlob(match matcher, visitor visitor, blob *common.DataBlob) (*common.DataBlob, bool, error) {
	if blob == nil || len(blob.Data) == 0 {
		return blob, false, nil
	}
	blobs, changed, err := translateDataBlobs(match, visitor, blob)
	if err != nil {
		return nil, changed, err
	}
	if len(blobs) != 1 {
		return nil, changed, fmt.Errorf("failed to translate single data blob")
	}
	return blobs[0], changed, err
}

func translateDataBlobs(match matcher, visitor visitor, blobs ...*common.DataBlob) ([]*common.DataBlob, bool, error) {
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
