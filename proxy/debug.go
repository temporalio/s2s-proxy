package proxy

import (
	"encoding/json"
	"net/http"
	"time"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type (
	// StreamInfo represents information about an active gRPC stream
	StreamInfo struct {
		ID          string    `json:"id"`
		Method      string    `json:"method"`
		Direction   string    `json:"direction"`
		ClientShard string    `json:"client_shard"`
		ServerShard string    `json:"server_shard"`
		StartTime   time.Time `json:"start_time"`
		LastSeen    time.Time `json:"last_seen"`
	}

	DebugResponse struct {
		Timestamp     time.Time    `json:"timestamp"`
		ActiveStreams []StreamInfo `json:"active_streams"`
		StreamCount   int          `json:"stream_count"`
	}
)

func HandleDebugInfo(w http.ResponseWriter, r *http.Request, logger log.Logger) {
	w.Header().Set("Content-Type", "application/json")

	var activeStreams []StreamInfo
	var streamCount int

	// Get active streams information
	streamTracker := GetGlobalStreamTracker()
	activeStreams = streamTracker.GetActiveStreams()
	streamCount = streamTracker.GetStreamCount()

	response := DebugResponse{
		Timestamp:     time.Now(),
		ActiveStreams: activeStreams,
		StreamCount:   streamCount,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode debug response", tag.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
