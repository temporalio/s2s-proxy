package proxy

import (
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

	streams := make([]StreamInfo, 0, len(st.streams))
	for _, stream := range st.streams {
		streams = append(streams, *stream)
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
