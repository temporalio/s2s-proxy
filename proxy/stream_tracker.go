package proxy

import (
	"fmt"
	"sync"
	"time"
)

// StreamTracker tracks active gRPC streams for debugging
type StreamTracker struct {
	mu      sync.RWMutex
	streams map[string]*StreamInfo
}

// NewStreamTracker creates a new stream tracker
func NewStreamTracker() *StreamTracker {
	return &StreamTracker{
		streams: make(map[string]*StreamInfo),
	}
}

// RegisterStream adds a new active stream
func (st *StreamTracker) RegisterStream(id, method, direction, clientShard, serverShard string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	st.streams[id] = &StreamInfo{
		ID:          id,
		Method:      method,
		Direction:   direction,
		ClientShard: clientShard,
		ServerShard: serverShard,
		StartTime:   now,
		LastSeen:    now,
	}
}

// UpdateStream updates the last seen time for a stream
func (st *StreamTracker) UpdateStream(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if stream, exists := st.streams[id]; exists {
		stream.LastSeen = time.Now()
	}
}

// UpdateStreamSyncReplicationState updates the sync replication state information for a stream
func (st *StreamTracker) UpdateStreamSyncReplicationState(id string, inclusiveLowWatermark int64, watermarkTime *time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if stream, exists := st.streams[id]; exists {
		stream.LastSeen = time.Now()
		stream.LastSyncWatermark = &inclusiveLowWatermark
		stream.LastSyncWatermarkTime = watermarkTime
	}
}

// UpdateStreamReplicationMessages updates the replication messages information for a stream
func (st *StreamTracker) UpdateStreamReplicationMessages(id string, exclusiveHighWatermark int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if stream, exists := st.streams[id]; exists {
		stream.LastSeen = time.Now()
		stream.LastExclusiveHighWatermark = &exclusiveHighWatermark
	}
}

// UnregisterStream removes a stream from tracking
func (st *StreamTracker) UnregisterStream(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	delete(st.streams, id)
}

// GetActiveStreams returns a copy of all active streams
func (st *StreamTracker) GetActiveStreams() []StreamInfo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	now := time.Now()
	streams := make([]StreamInfo, 0, len(st.streams))
	for _, stream := range st.streams {
		// Create a copy and calculate both durations in seconds
		streamCopy := *stream
		totalSeconds := int(now.Sub(stream.StartTime).Seconds())
		idleSeconds := int(now.Sub(stream.LastSeen).Seconds())
		streamCopy.TotalDuration = formatDurationSeconds(totalSeconds)
		streamCopy.IdleDuration = formatDurationSeconds(idleSeconds)
		streams = append(streams, streamCopy)
	}

	return streams
}

// GetStreamCount returns the number of active streams
func (st *StreamTracker) GetStreamCount() int {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return len(st.streams)
}

// Global stream tracker instance
var globalStreamTracker = NewStreamTracker()

// GetGlobalStreamTracker returns the global stream tracker instance
func GetGlobalStreamTracker() *StreamTracker {
	return globalStreamTracker
}

// formatDurationSeconds formats a duration in seconds to a readable string
func formatDurationSeconds(totalSeconds int) string {
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds", totalSeconds)
	}

	minutes := totalSeconds / 60
	seconds := totalSeconds % 60

	if minutes < 60 {
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}

	hours := minutes / 60
	minutes = minutes % 60

	if minutes == 0 && seconds == 0 {
		return fmt.Sprintf("%dh", hours)
	} else if seconds == 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	} else if minutes == 0 {
		return fmt.Sprintf("%dh%ds", hours, seconds)
	} else {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
}
