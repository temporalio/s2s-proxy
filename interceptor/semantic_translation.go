package interceptor

import (
	"sort"
	"sync"
	"time"

	adminservicepb "go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/failoverversion"
	"github.com/temporalio/s2s-proxy/metrics"
)

// TranslationReport describes what a successful semantic translation changed.
// It intentionally contains counts only, never translated values or request
// identifiers, so it is safe to use in metrics, debug state, and test output.
type TranslationReport struct {
	Changed          bool   `json:"changed"`
	VersionFields    uint32 `json:"version_fields"`
	FVIs             uint32 `json:"fvis"`
	Lanes            uint32 `json:"lanes"`
	ClusterIDs       uint32 `json:"cluster_ids"`
	HistoryBlobs     uint32 `json:"history_blobs"`
	StatusDetails    uint32 `json:"status_details"`
	ChecksumsCleared uint32 `json:"checksums_cleared"`
}

func (r *TranslationReport) merge(other TranslationReport) {
	r.Changed = r.Changed || other.Changed
	r.VersionFields += other.VersionFields
	r.FVIs += other.FVIs
	r.Lanes += other.Lanes
	r.ClusterIDs += other.ClusterIDs
	r.HistoryBlobs += other.HistoryBlobs
	r.StatusDetails += other.StatusDetails
	r.ChecksumsCleared += other.ChecksumsCleared
}

// TranslationResult is transactional: Message is a translated clone on
// success. On failure callers receive no result and the input remains intact.
type TranslationResult struct {
	Message proto.Message
	Report  TranslationReport
}

type ErrorTranslationResult struct {
	Error  error
	Report TranslationReport
}

type TranslationObservation struct {
	Kind                 string                           `json:"kind"`
	Direction            failoverversion.Direction        `json:"direction"`
	Operation            string                           `json:"operation"`
	MessageType          string                           `json:"message_type"`
	Outcome              string                           `json:"outcome"`
	ErrorCode            failoverversion.ErrorCode        `json:"error_code,omitempty"`
	FieldPath            string                           `json:"field_path,omitempty"`
	MappingFingerprint   string                           `json:"mapping_fingerprint,omitempty"`
	Report               TranslationReport                `json:"report"`
	Duration             time.Duration                    `json:"duration"`
	ObservedAt           time.Time                        `json:"observed_at"`
	NamespaceReplication *NamespaceReplicationObservation `json:"namespace_replication,omitempty"`
}

// NamespaceReplicationObservation exposes queue progress without namespace,
// payload, or version values. Cursor continuity is essential when a cluster is
// recreated under an existing name because a stale high cursor can skip every
// task in the fresh queue.
type NamespaceReplicationObservation struct {
	LastRetrievedMessageID int64 `json:"last_retrieved_message_id"`
	LastProcessedMessageID int64 `json:"last_processed_message_id,omitempty"`
	ResponseLastMessageID  int64 `json:"response_last_message_id,omitempty"`
	TaskCount              int   `json:"task_count,omitempty"`
}

func observeNamespaceReplication(message proto.Message) *NamespaceReplicationObservation {
	switch value := message.(type) {
	case *adminservicepb.GetNamespaceReplicationMessagesRequest:
		return &NamespaceReplicationObservation{
			LastRetrievedMessageID: value.GetLastRetrievedMessageId(),
			LastProcessedMessageID: value.GetLastProcessedMessageId(),
		}
	case *adminservicepb.GetNamespaceReplicationMessagesResponse:
		messages := value.GetMessages()
		if messages == nil {
			return &NamespaceReplicationObservation{}
		}
		return &NamespaceReplicationObservation{
			ResponseLastMessageID: messages.GetLastRetrievedMessageId(),
			TaskCount:             len(messages.GetReplicationTasks()),
		}
	default:
		return nil
	}
}

// TranslationOperationSummary is a cardinality-bounded, value-free lifetime
// view of one direction/operation pair. It makes low-frequency control-plane
// traffic observable even after the recent-observation ring has rolled over.
type TranslationOperationSummary struct {
	Direction      failoverversion.Direction `json:"direction"`
	Operation      string                    `json:"operation"`
	Successes      uint64                    `json:"successes"`
	Errors         uint64                    `json:"errors"`
	Changed        uint64                    `json:"changed"`
	LastOutcome    string                    `json:"last_outcome"`
	LastObservedAt time.Time                 `json:"last_observed_at"`
}

type TranslationObserver interface {
	Observe(TranslationObservation)
}

type TranslationObserverFunc func(TranslationObservation)

func (f TranslationObserverFunc) Observe(observation TranslationObservation) { f(observation) }

// TranslationDebugObserver retains only sanitized observations and has a
// bounded history so it is safe to attach to long-lived connections.
type TranslationDebugObserver struct {
	mu          sync.RWMutex
	limit       int
	latest      *TranslationObservation
	lastSuccess *TranslationObservation
	recent      []TranslationObservation
	failures    []TranslationObservation
	operations  map[string]TranslationOperationSummary
}

func NewTranslationDebugObserver(limit int) *TranslationDebugObserver {
	if limit < 1 {
		limit = 1
	}
	return &TranslationDebugObserver{limit: limit, operations: make(map[string]TranslationOperationSummary)}
}

func (o *TranslationDebugObserver) Observe(observation TranslationObservation) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	copy := observation
	key := string(observation.Direction) + "\x00" + observation.Operation
	summary := o.operations[key]
	summary.Direction = observation.Direction
	summary.Operation = observation.Operation
	summary.LastOutcome = observation.Outcome
	summary.LastObservedAt = observation.ObservedAt
	if observation.Outcome == "error" {
		summary.Errors++
	} else {
		summary.Successes++
	}
	if observation.Report.Changed {
		summary.Changed++
	}
	o.operations[key] = summary
	o.latest = &copy
	o.recent = append(o.recent, copy)
	if len(o.recent) > o.limit {
		o.recent = append([]TranslationObservation(nil), o.recent[len(o.recent)-o.limit:]...)
	}
	if observation.Outcome != "error" {
		o.lastSuccess = &copy
		return
	}
	o.failures = append(o.failures, copy)
	if len(o.failures) > o.limit {
		o.failures = append([]TranslationObservation(nil), o.failures[len(o.failures)-o.limit:]...)
	}
}

// OperationSummaries returns a deterministic snapshot of the lifetime
// counters. Operations are protocol method names, never request data.
func (o *TranslationDebugObserver) OperationSummaries() []TranslationOperationSummary {
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	summaries := make([]TranslationOperationSummary, 0, len(o.operations))
	for _, summary := range o.operations {
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Direction == summaries[j].Direction {
			return summaries[i].Operation < summaries[j].Operation
		}
		return summaries[i].Direction < summaries[j].Direction
	})
	return summaries
}

// SnapshotDetailed returns all bounded sanitized observations as well as the
// failure-only view retained for alert-focused consumers.
func (o *TranslationDebugObserver) SnapshotDetailed() (
	latest, lastSuccess *TranslationObservation,
	recent, failures []TranslationObservation,
) {
	if o == nil {
		return nil, nil, nil, nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.latest != nil {
		copy := *o.latest
		latest = &copy
	}
	if o.lastSuccess != nil {
		copy := *o.lastSuccess
		lastSuccess = &copy
	}
	return latest, lastSuccess,
		append([]TranslationObservation(nil), o.recent...),
		append([]TranslationObservation(nil), o.failures...)
}

func (o *TranslationDebugObserver) Snapshot() (latest, lastSuccess *TranslationObservation, failures []TranslationObservation) {
	if o == nil {
		return nil, nil, nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.latest != nil {
		copy := *o.latest
		latest = &copy
	}
	if o.lastSuccess != nil {
		copy := *o.lastSuccess
		lastSuccess = &copy
	}
	return latest, lastSuccess, append([]TranslationObservation(nil), o.failures...)
}

type compositeTranslationObserver []TranslationObserver

func (o compositeTranslationObserver) Observe(observation TranslationObservation) {
	for _, observer := range o {
		if observer != nil {
			observer.Observe(observation)
		}
	}
}

type prometheusTranslationObserver struct{}

func (prometheusTranslationObserver) Observe(observation TranslationObservation) {
	direction := string(observation.Direction)
	if direction == "" {
		direction = "unknown"
	}
	reason := string(observation.ErrorCode)
	if reason == "" {
		reason = "none"
	}
	metrics.FailoverVersionTranslationOperations.WithLabelValues(direction, observation.Outcome, reason).Inc()
	add := func(semantic string, count uint32) {
		if count != 0 {
			metrics.FailoverVersionTranslationFields.WithLabelValues(direction, semantic).Add(float64(count))
		}
	}
	add("version", observation.Report.VersionFields)
	add("fvi", observation.Report.FVIs)
	add("lane", observation.Report.Lanes)
	add("cluster_id", observation.Report.ClusterIDs)
	add("history_blob", observation.Report.HistoryBlobs)
	add("status_detail", observation.Report.StatusDetails)
	add("checksum_cleared", observation.Report.ChecksumsCleared)
}
