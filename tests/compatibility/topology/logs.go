//go:build testcompatibility

package topology

import (
	"context"
	"sync"

	"github.com/testcontainers/testcontainers-go"
)

// logConsumer buffers log lines for later retrieval via Logs().
type logConsumer struct {
	mu    sync.Mutex
	lines []string
}

func (c *logConsumer) Accept(l testcontainers.Log) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, string(l.Content))
}

// Logs returns all buffered log lines.
func (c *logConsumer) Logs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.lines))
	copy(result, c.lines)
	return result
}

// attachLogConsumer attaches a new logConsumer to a running container and starts log streaming.
func attachLogConsumer(ctx context.Context, c testcontainers.Container) *logConsumer {
	lc := &logConsumer{}
	c.FollowOutput(lc)
	_ = c.StartLogProducer(ctx)
	return lc
}
