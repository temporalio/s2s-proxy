package proxy

import (
	"encoding/json"
	"net/http"
	"time"

	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type (

	// ProxyIDEntry is a preview of a ring buffer entry
	ProxyIDEntry struct {
		ProxyID     int64  `json:"proxy_id"`
		SourceShard string `json:"source_shard"`
		SourceTask  int64  `json:"source_task"`
	}

	// SenderDebugInfo captures proxy-stream-sender internals for debugging
	SenderDebugInfo struct {
		RingStartProxyID      int64            `json:"ring_start_proxy_id"`
		RingSize              int              `json:"ring_size"`
		RingCapacity          int              `json:"ring_capacity"`
		RingHead              int              `json:"ring_head"`
		NextProxyTaskID       int64            `json:"next_proxy_task_id"`
		PrevAckBySource       map[string]int64 `json:"prev_ack_by_source"`
		LastHighBySource      map[string]int64 `json:"last_high_by_source"`
		LastProxyHighBySource map[string]int64 `json:"last_proxy_high_by_source"`
		EntriesPreview        []ProxyIDEntry   `json:"entries_preview"`
	}

	// ReceiverDebugInfo captures proxy-stream-receiver ack aggregation state
	ReceiverDebugInfo struct {
		AckByTarget               map[string]int64 `json:"ack_by_target"`
		LastAggregatedMin         int64            `json:"last_aggregated_min"`
		LastExclusiveHighOriginal int64            `json:"last_exclusive_high_original"`
	}

	// StreamInfo represents information about an active gRPC stream
	StreamInfo struct {
		ID                         string             `json:"id"`
		Method                     string             `json:"method"`
		Direction                  string             `json:"direction"`
		Role                       string             `json:"role"`
		ClientShard                string             `json:"client_shard"`
		ServerShard                string             `json:"server_shard"`
		StartTime                  time.Time          `json:"start_time"`
		LastSeen                   time.Time          `json:"last_seen"`
		TotalDuration              string             `json:"total_duration"`
		IdleDuration               string             `json:"idle_duration"`
		LastSyncWatermark          *int64             `json:"last_sync_watermark"`
		LastSyncWatermarkTime      *time.Time         `json:"last_sync_watermark_time"`
		LastExclusiveHighWatermark *int64             `json:"last_exclusive_high_watermark"`
		LastTaskIDs                []int64            `json:"last_task_ids"`
		SenderDebug                *SenderDebugInfo   `json:"sender_debug"`
		ReceiverDebug              *ReceiverDebugInfo `json:"receiver_debug"`
	}

	// ShardDebugInfo contains debug information about shard distribution
	ShardDebugInfo struct {
		Enabled           bool                     `json:"enabled"`
		ForwardingEnabled bool                     `json:"forwarding_enabled"`
		NodeName          string                   `json:"node_name"`
		LocalShards       []history.ClusterShardID `json:"local_shards"`
		LocalShardCount   int                      `json:"local_shard_count"`
		ClusterNodes      []string                 `json:"cluster_nodes"`
		ClusterSize       int                      `json:"cluster_size"`
		RemoteShards      map[string]string        `json:"remote_shards"`       // shard_id -> node_name
		RemoteShardCounts map[string]int           `json:"remote_shard_counts"` // node_name -> shard_count
	}

	// ChannelDebugInfo holds debug information about channels
	ChannelDebugInfo struct {
		RemoteSendChannels map[string]int `json:"remote_send_channels"` // shard ID -> buffer size
		LocalAckChannels   map[string]int `json:"local_ack_channels"`   // shard ID -> buffer size
		TotalSendChannels  int            `json:"total_send_channels"`
		TotalAckChannels   int            `json:"total_ack_channels"`
	}

	DebugResponse struct {
		Timestamp     time.Time        `json:"timestamp"`
		ActiveStreams []StreamInfo     `json:"active_streams"`
		StreamCount   int              `json:"stream_count"`
		ShardInfo     ShardDebugInfo   `json:"shard_info"`
		ChannelInfo   ChannelDebugInfo `json:"channel_info"`
	}
)

func HandleDebugInfo(w http.ResponseWriter, r *http.Request, proxyInstance *Proxy, logger log.Logger) {
	w.Header().Set("Content-Type", "application/json")

	var activeStreams []StreamInfo
	var streamCount int
	var shardInfo ShardDebugInfo
	var channelInfo ChannelDebugInfo

	// Get active streams information
	streamTracker := GetGlobalStreamTracker()
	activeStreams = streamTracker.GetActiveStreams()
	streamCount = streamTracker.GetStreamCount()
	shardInfo = proxyInstance.GetShardInfo()
	channelInfo = proxyInstance.GetChannelInfo()

	response := DebugResponse{
		Timestamp:     time.Now(),
		ActiveStreams: activeStreams,
		StreamCount:   streamCount,
		ShardInfo:     shardInfo,
		ChannelInfo:   channelInfo,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode debug response", tag.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
