package proxy

import (
	"fmt"
	"sync"
	"time"

	"go.temporal.io/server/client/history"
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

// BuildSenderStreamID returns the canonical sender stream ID.
func BuildSenderStreamID(source, target history.ClusterShardID) string {
	return fmt.Sprintf("snd-%s", ClusterShardIDtoShortString(target))
}

// BuildReceiverStreamID returns the canonical receiver stream ID.
func BuildReceiverStreamID(source, target history.ClusterShardID) string {
	return fmt.Sprintf("rcv-%s", ClusterShardIDtoShortString(source))
}

// BuildForwarderStreamID returns the canonical forwarder stream ID.
// Note: forwarder uses server-first ordering in the ID.
func BuildForwarderStreamID(client, server history.ClusterShardID) string {
	return fmt.Sprintf("fwd-snd-%s", ClusterShardIDtoShortString(server))
}

// BuildIntraProxySenderStreamID returns the server-side intra-proxy stream ID for a peer and shard pair.
func BuildIntraProxySenderStreamID(peer string, source, target history.ClusterShardID) string {
	return fmt.Sprintf("ip-snd-%s-%s|%s", ClusterShardIDtoShortString(source), ClusterShardIDtoShortString(target), peer)
}

// BuildIntraProxyReceiverStreamID returns the client-side intra-proxy stream ID for a peer and shard pair.
func BuildIntraProxyReceiverStreamID(peer string, source, target history.ClusterShardID) string {
	return fmt.Sprintf("ip-rcv-%s-%s|%s", ClusterShardIDtoShortString(source), ClusterShardIDtoShortString(target), peer)
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
