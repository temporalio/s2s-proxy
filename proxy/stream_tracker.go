package proxy

import (
	"fmt"
	"sync"
	"time"
)

const (
	StreamRoleSender    = "Sender"
	StreamRoleReceiver  = "Receiver"
	StreamRoleForwarder = "Forwarder"
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
func (st *StreamTracker) RegisterStream(id, method, direction, clientShard, serverShard, role string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	st.streams[id] = &StreamInfo{
		ID:            id,
		Method:        method,
		Direction:     direction,
		ClientShard:   clientShard,
		ServerShard:   serverShard,
		Role:          role,
		StartTime:     now,
		LastSeen:      now,
		SenderDebug:   &SenderDebugInfo{},
		ReceiverDebug: &ReceiverDebugInfo{},
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

// UpdateStreamLastTaskIDs updates the last seen task ids for a stream
func (st *StreamTracker) UpdateStreamLastTaskIDs(id string, taskIDs []int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if stream, exists := st.streams[id]; exists {
		stream.LastSeen = time.Now()
		stream.LastTaskIDs = taskIDs
	}
}

// UnregisterStream removes a stream from tracking
func (st *StreamTracker) UnregisterStream(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	delete(st.streams, id)
}

// SenderDebugInfo captures proxy-stream-sender internals for debugging
type SenderDebugInfo struct {
	RingStartProxyID      int64            `json:"ring_start_proxy_id,omitempty"`
	RingSize              int              `json:"ring_size,omitempty"`
	RingCapacity          int              `json:"ring_capacity,omitempty"`
	RingHead              int              `json:"ring_head,omitempty"`
	NextProxyTaskID       int64            `json:"next_proxy_task_id,omitempty"`
	PrevAckBySource       map[string]int64 `json:"prev_ack_by_source,omitempty"`
	LastHighBySource      map[string]int64 `json:"last_high_by_source,omitempty"`
	LastProxyHighBySource map[string]int64 `json:"last_proxy_high_by_source,omitempty"`
	EntriesPreview        []ProxyIDEntry   `json:"entries_preview,omitempty"`
}

// ProxyIDEntry is a preview of a ring buffer entry
type ProxyIDEntry struct {
	ProxyID     int64  `json:"proxy_id"`
	SourceShard string `json:"source_shard"`
	SourceTask  int64  `json:"source_task"`
}

// ReceiverDebugInfo captures proxy-stream-receiver ack aggregation state
type ReceiverDebugInfo struct {
	AckByTarget               map[string]int64 `json:"ack_by_target,omitempty"`
	LastAggregatedMin         int64            `json:"last_aggregated_min,omitempty"`
	LastExclusiveHighOriginal int64            `json:"last_exclusive_high_original,omitempty"`
}

// UpdateStreamSenderDebug sets the sender debug snapshot for a stream
func (st *StreamTracker) UpdateStreamSenderDebug(id string, info *SenderDebugInfo) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if stream, exists := st.streams[id]; exists {
		stream.SenderDebug = info
		stream.LastSeen = time.Now()
	}
}

// UpdateStreamReceiverDebug sets the receiver debug snapshot for a stream
func (st *StreamTracker) UpdateStreamReceiverDebug(id string, info *ReceiverDebugInfo) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if stream, exists := st.streams[id]; exists {
		stream.ReceiverDebug = info
		stream.LastSeen = time.Now()
	}
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
