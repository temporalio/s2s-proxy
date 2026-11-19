package proxy

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sort"
	"time"

	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/failoverversion"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

// HandleDebugInfo is the HTTP handler for the proxy debug endpoint.
// It returns JSON-encoded debug information including active streams, shard distribution,
// channel states, and mux connection details.

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
		RingMaxSize           int              `json:"ring_max_size"`
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
		Enabled           bool                              `json:"enabled"`
		NodeName          string                            `json:"node_name"`
		LocalShards       map[string]history.ClusterShardID `json:"local_shards"` // key: "clusterID:shardID"
		LocalShardCount   int                               `json:"local_shard_count"`
		ClusterNodes      []string                          `json:"cluster_nodes"`
		ClusterSize       int                               `json:"cluster_size"`
		RemoteShards      map[string]string                 `json:"remote_shards"`       // shard_id -> node_name
		RemoteShardCounts map[string]int                    `json:"remote_shard_counts"` // node_name -> shard_count
	}

	// ChannelDebugInfo holds debug information about channels
	ChannelDebugInfo struct {
		RemoteSendChannels map[string]int `json:"remote_send_channels"` // shard ID -> buffer size
		LocalAckChannels   map[string]int `json:"local_ack_channels"`   // shard ID -> buffer size
		TotalSendChannels  int            `json:"total_send_channels"`
		TotalAckChannels   int            `json:"total_ack_channels"`
	}

	// MuxConnectionInfo holds debug information about a mux connection
	MuxConnectionInfo struct {
		ID         string `json:"id"`
		LocalAddr  string `json:"local_addr"`
		RemoteAddr string `json:"remote_addr"`
		State      string `json:"state"`
		IsClosed   bool   `json:"is_closed"`
	}

	// MuxConnectionsDebugInfo holds debug information about mux connections for a cluster connection
	MuxConnectionsDebugInfo struct {
		ConnectionName  string              `json:"connection_name"`
		Direction       string              `json:"direction"`
		Address         string              `json:"address"`
		Connections     []MuxConnectionInfo `json:"connections"`
		ConnectionCount int                 `json:"connection_count"`
	}

	// FailoverVersionTranslationDebugInfo exposes the safe shape and runtime
	// observations of a connection's semantic translation. Individual lane
	// pairs are deliberately represented only by the canonical fingerprint.
	FailoverVersionTranslationDebugInfo struct {
		ConnectionName            string                                    `json:"connection_name"`
		Enabled                   bool                                      `json:"enabled"`
		Role                      string                                    `json:"role"`
		Mapping                   failoverversion.MappingDescriptor         `json:"mapping,omitempty"`
		OutboundRequestDirection  failoverversion.Direction                 `json:"outbound_request_direction,omitempty"`
		OutboundResponseDirection failoverversion.Direction                 `json:"outbound_response_direction,omitempty"`
		InboundRequestDirection   failoverversion.Direction                 `json:"inbound_request_direction,omitempty"`
		InboundResponseDirection  failoverversion.Direction                 `json:"inbound_response_direction,omitempty"`
		Latest                    *interceptor.TranslationObservation       `json:"latest,omitempty"`
		LastSuccess               *interceptor.TranslationObservation       `json:"last_success,omitempty"`
		Recent                    []interceptor.TranslationObservation      `json:"recent,omitempty"`
		RecentFailures            []interceptor.TranslationObservation      `json:"recent_failures,omitempty"`
		OperationSummaries        []interceptor.TranslationOperationSummary `json:"operation_summaries,omitempty"`
	}

	RuntimeMemoryDebugInfo struct {
		HeapAlloc    uint64 `json:"heap_alloc_bytes"`
		HeapInuse    uint64 `json:"heap_inuse_bytes"`
		HeapReleased uint64 `json:"heap_released_bytes"`
		HeapObjects  uint64 `json:"heap_objects"`
		StackInuse   uint64 `json:"stack_inuse_bytes"`
		Sys          uint64 `json:"sys_bytes"`
		NumGC        uint32 `json:"num_gc"`
		Goroutines   int    `json:"goroutines"`
	}

	DebugResponse struct {
		Timestamp                   time.Time                             `json:"timestamp"`
		ActiveStreams               []StreamInfo                          `json:"active_streams"`
		StreamCount                 int                                   `json:"stream_count"`
		ShardInfos                  []ShardDebugInfo                      `json:"shard_infos"`
		ChannelInfos                []ChannelDebugInfo                    `json:"channel_infos"`
		MuxConnections              []MuxConnectionsDebugInfo             `json:"mux_connections"`
		FailoverVersionTranslations []FailoverVersionTranslationDebugInfo `json:"failover_version_translations"`
		RuntimeMemory               RuntimeMemoryDebugInfo                `json:"runtime_memory"`
	}
)

func HandleDebugInfo(w http.ResponseWriter, r *http.Request, proxyInstance *Proxy, logger log.Logger) {
	w.Header().Set("Content-Type", "application/json")

	var activeStreams []StreamInfo
	var streamCount int
	var shardInfos []ShardDebugInfo
	var channelInfos []ChannelDebugInfo
	var muxConnections []MuxConnectionsDebugInfo
	var failoverVersionTranslations []FailoverVersionTranslationDebugInfo

	// Get active streams information
	streamTracker := GetGlobalStreamTracker()
	activeStreams = streamTracker.GetActiveStreams()
	streamCount = streamTracker.GetStreamCount()
	for id, clusterConnection := range proxyInstance.clusterConnections {
		if clusterConnection.shardManager != nil {
			shardInfos = append(shardInfos, clusterConnection.shardManager.GetShardInfos()...)
			channelInfos = append(channelInfos, clusterConnection.shardManager.GetChannelInfo())
		}

		// Collect mux connection info from inbound and outbound servers
		muxConnections = append(muxConnections, getMuxConnectionsInfo(clusterConnection.inboundServer, "inbound")...)
		muxConnections = append(muxConnections, getMuxConnectionsInfo(clusterConnection.outboundServer, "outbound")...)

		translation := FailoverVersionTranslationDebugInfo{ConnectionName: id.name, Role: "pass_through"}
		if clusterConnection.translationDescriptor != nil {
			translation.Enabled = true
			translation.Role = "owner"
			translation.Mapping = *clusterConnection.translationDescriptor
			translation.OutboundRequestDirection = failoverversion.LocalToRemote
			translation.OutboundResponseDirection = failoverversion.RemoteToLocal
			translation.InboundRequestDirection = failoverversion.RemoteToLocal
			translation.InboundResponseDirection = failoverversion.LocalToRemote
			translation.Latest, translation.LastSuccess, translation.Recent, translation.RecentFailures =
				clusterConnection.translationObserver.SnapshotDetailed()
			translation.OperationSummaries = clusterConnection.translationObserver.OperationSummaries()
		}
		failoverVersionTranslations = append(failoverVersionTranslations, translation)
	}
	sort.Slice(failoverVersionTranslations, func(i, j int) bool {
		return failoverVersionTranslations[i].ConnectionName < failoverVersionTranslations[j].ConnectionName
	})
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	response := DebugResponse{
		Timestamp:                   time.Now(),
		ActiveStreams:               activeStreams,
		StreamCount:                 streamCount,
		ShardInfos:                  shardInfos,
		ChannelInfos:                channelInfos,
		MuxConnections:              muxConnections,
		FailoverVersionTranslations: failoverVersionTranslations,
		RuntimeMemory: RuntimeMemoryDebugInfo{
			HeapAlloc: memory.HeapAlloc, HeapInuse: memory.HeapInuse,
			HeapReleased: memory.HeapReleased, HeapObjects: memory.HeapObjects,
			StackInuse: memory.StackInuse, Sys: memory.Sys, NumGC: memory.NumGC,
			Goroutines: runtime.NumGoroutine(),
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode debug response", tag.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func getMuxConnectionsInfo(server contextAwareServer, direction string) []MuxConnectionsDebugInfo {
	muxMgr, ok := server.(mux.MultiMuxManager)
	if !ok {
		return nil
	}

	connections := muxMgr.GetMuxConnections()
	if len(connections) == 0 {
		return nil
	}

	var connInfos []MuxConnectionInfo
	for id, muxSession := range connections {
		localAddr, remoteAddr := muxSession.GetConnectionInfo()
		stateStr := mux.SessionStateName(muxSession.State())

		localAddrStr := ""
		if localAddr != nil {
			localAddrStr = localAddr.String()
		}
		remoteAddrStr := ""
		if remoteAddr != nil {
			remoteAddrStr = remoteAddr.String()
		}

		connInfos = append(connInfos, MuxConnectionInfo{
			ID:         id,
			LocalAddr:  localAddrStr,
			RemoteAddr: remoteAddrStr,
			State:      stateStr,
			IsClosed:   muxSession.IsClosed(),
		})
	}

	return []MuxConnectionsDebugInfo{
		{
			ConnectionName:  muxMgr.Name(),
			Direction:       direction,
			Address:         muxMgr.Address(),
			Connections:     connInfos,
			ConnectionCount: len(connInfos),
		},
	}
}
