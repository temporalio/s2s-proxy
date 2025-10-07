package proxy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

func TestAdminServiceObserver(t *testing.T) {
	observer := NewReplicationStreamObserver(log.NewTestLogger())
	observer.ReportStreamValue(1, 1)
	require.Equal(t, "[1,]", observer.PrintActiveStreams())
	observer.ReportStreamValue(1, -1)
	require.Equal(t, "[]", observer.PrintActiveStreams())
}

func TestAdminServiceObserver_Grows(t *testing.T) {
	observer := NewReplicationStreamObserver(log.NewTestLogger())
	observer.ReportStreamValue(16_000, 1)
	require.Equal(t, "[16000,]", observer.PrintActiveStreams())
	observer.ReportStreamValue(16_000, -1)
	require.Equal(t, "[]", observer.PrintActiveStreams())
}

func TestAdminServiceObserver_Invalid(t *testing.T) {
	observer := NewReplicationStreamObserver(log.NewTestLogger())
	observer.ReportStreamValue(-1, 1)
	require.Equal(t, "[]", observer.PrintActiveStreams())
}

type mockLogger struct {
	logs []logLine
	sync.Mutex
}
type logLine struct {
	msg  string
	tags []tag.Tag
}

func (l *mockLogger) Info(msg string, tags ...tag.Tag) {
	l.Lock()
	l.logs = append(l.logs, logLine{msg, tags})
	l.Unlock()
}
func (l *mockLogger) Warn(msg string, tags ...tag.Tag) {
	l.Lock()
	l.logs = append(l.logs, logLine{msg, tags})
	l.Unlock()
}

func TestAdminServiceObserver_GoRoutine(t *testing.T) {
	logOutput := &mockLogger{}
	observer := &ReplicationStreamObserver{
		streamActive:   make([]atomic.Int32, 0),
		streamGrowLock: sync.Mutex{},
		interval:       100 * time.Millisecond,
		logger:         logOutput,
	}
	observer.ReportStreamValue(1, 1)
	ctx, cancel := context.WithCancel(t.Context())
	observer.Start(ctx, "unittest", "testbound")
	time.Sleep(20 * time.Millisecond)
	observer.ReportStreamValue(16_000, 1)
	time.Sleep(100 * time.Millisecond)
	observer.ReportStreamValue(16_000, -1)
	time.Sleep(100 * time.Millisecond)
	cancel()
	logOutput.Lock()
	require.Len(t, logOutput.logs, 3)
	assert.Equal(t, logLine{"Logging active AdminService history streams",
		[]tag.Tag{tag.NewStringTag("direction", "testbound"), tag.NewStringTag("name", "unittest"),
			tag.NewStringTag("activeStreams", "[1,]")}}, logOutput.logs[0])
	assert.Equal(t, logLine{"Logging active AdminService history streams",
		[]tag.Tag{tag.NewStringTag("direction", "testbound"), tag.NewStringTag("name", "unittest"),
			tag.NewStringTag("activeStreams", "[1,16000,]")}}, logOutput.logs[1])
	assert.Equal(t, logLine{"Logging active AdminService history streams",
		[]tag.Tag{tag.NewStringTag("direction", "testbound"), tag.NewStringTag("name", "unittest"),
			tag.NewStringTag("activeStreams", "[1,]")}}, logOutput.logs[2])
	logOutput.Unlock()
}
