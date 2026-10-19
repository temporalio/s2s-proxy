package proxy

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

// errorRecorder keeps what is logged at Error. A nil embedded Logger means any
// other level panics rather than being quietly dropped.
type errorRecorder struct {
	log.Logger
	msgs []string
	tags [][]tag.Tag
}

func (r *errorRecorder) Error(msg string, tags ...tag.Tag) {
	r.msgs = append(r.msgs, msg)
	r.tags = append(r.tags, tags)
}

// The caller of a unary admin call failed this way only gets a generic status,
// so the full error has to reach the log.
func TestVisitErrorLoggerLogsTheFullError(t *testing.T) {
	rec := &errorRecorder{}
	newVisitErrorLogger(rec).VisitFailed("/AdminService/GetDLQReplicationMessages", errors.New(`failed to resolve namespace ID "id-a"`))

	require.Len(t, rec.msgs, 1)
	values := map[string]any{}
	for _, tg := range rec.tags[0] {
		values[tg.Key()] = tg.Value()
	}
	require.Equal(t, "/AdminService/GetDLQReplicationMessages", values["operation"])
	require.Contains(t, fmt.Sprint(values["error"]), "id-a")
}
