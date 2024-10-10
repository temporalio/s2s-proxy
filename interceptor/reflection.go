package interceptor

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/keilerkonzept/visit"
	replicationspb "go.temporal.io/server/api/replication/v1"
)

const (
	namespaceTaskAttributesFieldName = "NamespaceTaskAttributes"
)

var (
	namespaceFieldNames = []string{
		"Namespace",
		"WorkflowNamespace", // PollActivityTaskQueueResponse
	}
)

type matchHandler func(name string) (string, bool)

// visitNamespace uses reflection to recursively visit all fields
// in the given object. When it finds namespace string fields, it maps
// them to a new name, according to the mapping.
func visitNamespace(obj any, match matchHandler) (bool, error) {
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
		} else if vwp.Kind() == reflect.String && slices.Contains(namespaceFieldNames, fieldType.Name) {
			newName, ok := match(vwp.String())
			if !ok {
				return visit.Continue, nil
			}

			if err := visit.Assign(vwp, reflect.ValueOf(newName)); err != nil {
				return visit.Stop, err
			}
			matched = matched || ok
		}

		return visit.Continue, nil
	})
	return matched, err
}
