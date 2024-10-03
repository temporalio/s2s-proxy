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

// translateNamespace uses reflection to recursively visit all fields
// in the given object. When it finds namespace string fields, it maps
// them to a new name, according to the mapping.
func translateNamespace(obj any, mapping map[string]string) (bool, error) {
	var changed bool

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

			// Translate the namespace name.
			old := attrs.Info.Name
			new, ok := mapping[old]
			if !ok {
				return visit.Continue, nil
			}
			attrs.Info.Name = new
			changed = changed || old != new
		} else if vwp.Kind() == reflect.String && slices.Contains(namespaceFieldNames, fieldType.Name) {
			// Translate namespace struct fields.
			old := vwp.String()
			new, ok := mapping[old]
			if !ok {
				return visit.Continue, nil
			}
			if err := visit.Assign(vwp, reflect.ValueOf(new)); err != nil {
				return visit.Stop, err
			}
			changed = changed || old != new
		}

		return visit.Continue, nil
	})
	return changed, err
}
