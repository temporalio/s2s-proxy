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

	// StreamInfo represents information about an active gRPC stream
	StreamInfo struct {
		ID                         string             `json:"id"`
		Method                     string             `json:"method"`
		Direction                  string             `json:"direction"`
		Role                       string             `json:"role,omitempty"`
		ClientShard                string             `json:"client_shard"`
		ServerShard                string             `json:"server_shard"`
		StartTime                  time.Time          `json:"start_time"`
		LastSeen                   time.Time          `json:"last_seen"`
		TotalDuration              string             `json:"total_duration"`
		IdleDuration               string             `json:"idle_duration"`
		LastSyncWatermark          *int64             `json:"last_sync_watermark,omitempty"`
		LastSyncWatermarkTime      *time.Time         `json:"last_sync_watermark_time,omitempty"`
		LastExclusiveHighWatermark *int64             `json:"last_exclusive_high_watermark,omitempty"`
		LastTaskIDs                []int64            `json:"last_task_ids"`
		SenderDebug                *SenderDebugInfo   `json:"sender_debug,omitempty"`
		ReceiverDebug              *ReceiverDebugInfo `json:"receiver_debug,omitempty"`
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
