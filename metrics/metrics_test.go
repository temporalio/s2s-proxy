package metrics

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSanitizeForPrometheus(t *testing.T) {
	assert.Equal(t, "myCoolMetric_name:123", SanitizeForPrometheus("myCoolMetric_name:123"), "Overly aggressive replacement! We mangled a valid metric.")
	assert.Equal(t, "_23myCoolMetric_name:123", SanitizeForPrometheus("123myCoolMetric_name:123"), "Metrics are not allowed to start with a number")
	assert.Equal(t, ":weirdName:123", SanitizeForPrometheus(":weirdName:123"), "Metrics ARE allowed to start with a colon!")
	assert.Equal(t, "my_Cool_Metric_Name_:456", SanitizeForPrometheus("my@Cool#Metric-Name+:456"), "Underscore and colon are the only allowed special characters")
}
