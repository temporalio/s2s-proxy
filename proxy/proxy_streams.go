package proxy

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.temporal.io/server/api/adminservice/v1"
	replicationv1 "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	servercommon "go.temporal.io/server/common"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/metadata"
)

// proxyIDMapping stores the original source shard and task for a given proxy task ID
// Entries are kept in strictly increasing proxyID order.
type proxyIDMapping struct {
	sourceShard history.ClusterShardID
	sourceTask  int64
}

// proxyIDRingBuffer is a dynamically growing ring buffer keyed by monotonically increasing proxy IDs.
// It supports O(1) append and O(k) pop up to a given watermark, while preserving insertion order.
type proxyIDRingBuffer struct {
	entries      []proxyIDMapping
	head         int
	size         int
	startProxyID int64 // proxyID of the current head element when size > 0
}

func newProxyIDRingBuffer(capacity int) *proxyIDRingBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &proxyIDRingBuffer{entries: make([]proxyIDMapping, capacity)}
}

// ensureCapacity grows the buffer if it is full, preserving order.
func (b *proxyIDRingBuffer) ensureCapacity() {
	if b.size < len(b.entries) {
		return
	}
	newCap := len(b.entries) * 2
	if newCap == 0 {
		newCap = 1
	}
	newEntries := make([]proxyIDMapping, newCap)
	// copy existing elements in order starting from head
	for i := 0; i < b.size; i++ {
		idx := (b.head + i) % len(b.entries)
		newEntries[i] = b.entries[idx]
	}
	b.entries = newEntries
	b.head = 0
}

// Append appends a mapping for the given proxyID. ProxyIDs must be strictly increasing and contiguous.
func (b *proxyIDRingBuffer) Append(proxyID int64, sourceShard history.ClusterShardID, sourceTask int64) {
	b.ensureCapacity()
	if b.size == 0 {
		b.startProxyID = proxyID
	} else {
		// Maintain contiguity: next proxyID must be startProxyID + size
		expected := b.startProxyID + int64(b.size)
		if proxyID != expected {
			// If contiguity is violated, grow holes by inserting empty mappings until aligned.
			// In practice proxyID is always increasing by 1, so this branch should not trigger.
			for expected < proxyID {
				b.ensureCapacity()
				pos := (b.head + b.size) % len(b.entries)
				b.entries[pos] = proxyIDMapping{sourceShard: history.ClusterShardID{}, sourceTask: 0}
				b.size++
				expected++
			}
		}
	}
	pos := (b.head + b.size) % len(b.entries)
	b.entries[pos] = proxyIDMapping{sourceShard: sourceShard, sourceTask: sourceTask}
	b.size++
}

// PopUpTo pops and aggregates mappings up to and including the given watermark (proxy ID).
// Returns per-source-shard the maximal original source task acknowledged.
func (b *proxyIDRingBuffer) PopUpTo(watermark int64) map[history.ClusterShardID]int64 {
	result := make(map[history.ClusterShardID]int64)
	if b.size == 0 {
		return result
	}
	// if watermark is before head, nothing to pop
	if watermark < b.startProxyID {
		return result
	}
	count64 := watermark - b.startProxyID + 1
	if count64 <= 0 {
		return result
	}
	count := int(count64)
	if count > b.size {
		count = b.size
	}
	for i := 0; i < count; i++ {
		idx := (b.head + i) % len(b.entries)
		m := b.entries[idx]
		// Skip zero entries (shouldn't happen unless contiguity fix inserted holes)
		if m.sourceShard.ClusterID == 0 && m.sourceShard.ShardID == 0 {
			continue
		}
		if current, ok := result[m.sourceShard]; !ok || m.sourceTask > current {
			result[m.sourceShard] = m.sourceTask
		}
	}
	// advance head
	b.head = (b.head + count) % len(b.entries)
	b.size -= count
	b.startProxyID += int64(count)
	return result
}

// AggregateUpTo computes the per-shard aggregation up to watermark without removing entries.
// Returns (aggregation, count) where count is the number of entries covered.
func (b *proxyIDRingBuffer) AggregateUpTo(watermark int64) (map[history.ClusterShardID]int64, int) {
	result := make(map[history.ClusterShardID]int64)
	if b.size == 0 {
		return result, 0
	}
	if watermark < b.startProxyID {
		return result, 0
	}
	count64 := watermark - b.startProxyID + 1
	if count64 <= 0 {
		return result, 0
	}
	count := int(count64)
	if count > b.size {
		count = b.size
	}
	for i := 0; i < count; i++ {
		idx := (b.head + i) % len(b.entries)
		m := b.entries[idx]
		if m.sourceShard.ClusterID == 0 && m.sourceShard.ShardID == 0 {
			continue
		}
		if current, ok := result[m.sourceShard]; !ok || m.sourceTask > current {
			result[m.sourceShard] = m.sourceTask
		}
	}
	return result, count
}

// Discard advances the head by count entries, effectively removing them.
func (b *proxyIDRingBuffer) Discard(count int) {
	if count <= 0 {
		return
	}
	if count > b.size {
		count = b.size
	}
	b.head = (b.head + count) % len(b.entries)
	b.size -= count
	b.startProxyID += int64(count)
}

// proxyStreamSender is responsible for sending replication messages to the next hop
// (another proxy or a target server) and receiving ACKs back.
// This is scaffolding only – the concrete behavior will be wired in later.
type proxyStreamSender struct {
	logger         log.Logger
	shardManager   ShardManager
	proxy          *Proxy
	targetShardID  history.ClusterShardID
	sourceShardID  history.ClusterShardID
	directionLabel string
	streamID       string
	streamTracker  *StreamTracker
	// sendMsgChan carries replication messages to be sent to the remote side.
	sendMsgChan chan RoutedMessage

	mu              sync.Mutex
	nextProxyTaskID int64
	idRing          *proxyIDRingBuffer
	// prevAckBySource tracks the last ack level sent per original source shard
	prevAckBySource map[history.ClusterShardID]int64
}

// buildSenderDebugSnapshot returns a snapshot of the sender's ring buffer and related state
func (s *proxyStreamSender) buildSenderDebugSnapshot(maxEntries int) *SenderDebugInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := &SenderDebugInfo{
		PrevAckBySource: make(map[string]int64),
	}

	info.NextProxyTaskID = s.nextProxyTaskID

	for k, v := range s.prevAckBySource {
		info.PrevAckBySource[ClusterShardIDtoString(k)] = v
	}

	if s.idRing != nil {
		info.RingStartProxyID = s.idRing.startProxyID
		info.RingSize = s.idRing.size
		info.RingCapacity = len(s.idRing.entries)
		info.RingHead = s.idRing.head

		// Build entries preview
		if maxEntries <= 0 {
			maxEntries = 20
		}
		limit := s.idRing.size
		if limit > maxEntries {
			limit = maxEntries
		}
		info.EntriesPreview = make([]ProxyIDEntry, 0, limit)
		for i := 0; i < limit; i++ {
			idx := (s.idRing.head + i) % len(s.idRing.entries)
			e := s.idRing.entries[idx]
			info.EntriesPreview = append(info.EntriesPreview, ProxyIDEntry{
				ProxyID:     s.idRing.startProxyID + int64(i),
				SourceShard: ClusterShardIDtoString(e.sourceShard),
				SourceTask:  e.sourceTask,
			})
		}
	}

	return info
}

func (s *proxyStreamSender) Run(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	shutdownChan channel.ShutdownOnce,
) {
	s.logger = log.With(s.logger,
		tag.NewStringTag("role", "sender"),
	)
	s.logger.Info("proxyStreamSender Run")
	defer s.logger.Info("proxyStreamSender Run finished")

	// Register this sender as the owner of the shard for the duration of the stream
	s.shardManager.RegisterShard(s.targetShardID)
	defer s.shardManager.UnregisterShard(s.targetShardID)

	// Register local stream tracking for sender (short id, include role)
	s.streamTracker = GetGlobalStreamTracker()
	s.streamID = BuildSenderStreamID(s.sourceShardID, s.targetShardID)
	s.logger = log.With(s.logger, tag.NewStringTag("streamID", s.streamID))
	s.streamTracker.RegisterStream(
		s.streamID,
		"StreamWorkflowReplicationMessages",
		s.directionLabel,
		ClusterShardIDtoString(s.sourceShardID),
		ClusterShardIDtoString(s.targetShardID),
		StreamRoleSender,
	)
	defer s.streamTracker.UnregisterStream(s.streamID)

	wg := sync.WaitGroup{}
	// lazy init maps
	s.mu.Lock()
	if s.idRing == nil {
		s.idRing = newProxyIDRingBuffer(1024)
	}
	if s.prevAckBySource == nil {
		s.prevAckBySource = make(map[history.ClusterShardID]int64)
	}
	s.mu.Unlock()

	// Register remote send channel for this shard so receiver can forward tasks locally
	s.sendMsgChan = make(chan RoutedMessage, 100)

	s.proxy.SetRemoteSendChan(s.targetShardID, s.sendMsgChan)
	defer s.proxy.RemoveRemoteSendChan(s.targetShardID)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = s.sendReplicationMessages(targetStreamServer, shutdownChan)
	}()
	go func() {
		defer wg.Done()
		_ = s.recvAck(targetStreamServer, shutdownChan)
	}()
	// Wait for shutdown signal (triggered by receiver or stream errors)
	<-shutdownChan.Channel()
	// Ensure send loop exits promptly
	close(s.sendMsgChan)
	// Do not block waiting for ack goroutine; it will terminate when stream ends
}

// recvAck receives ACKs from the remote side and forwards them to the provided
// channel for aggregation/routing. Non-blocking shutdown is coordinated via
// shutdownChan. This is a placeholder implementation.
func (s *proxyStreamSender) recvAck(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	shutdownChan channel.ShutdownOnce,
) error {
	defer func() {
		s.logger.Info("Shutdown targetStreamServer.Recv loop.")
		shutdownChan.Shutdown()
	}()
	for !shutdownChan.IsShutdown() {
		req, err := targetStreamServer.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Unmap proxy task IDs back to original source shard/task and ACK by source shard
		if attr, ok := req.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState); ok && attr.SyncReplicationState != nil {
			proxyAckWatermark := attr.SyncReplicationState.InclusiveLowWatermark

			// Log incoming upstream ACK watermark
			s.logger.Info("Sender received upstream ACK", tag.NewInt64("inclusive_low", proxyAckWatermark))
			// track sync watermark
			s.streamTracker.UpdateStreamSyncReplicationState(s.streamID, proxyAckWatermark, nil)
			s.streamTracker.UpdateStream(s.streamID)

			s.mu.Lock()
			shardToAck, pendingDiscard := s.idRing.AggregateUpTo(proxyAckWatermark)
			s.mu.Unlock()

			if len(shardToAck) > 0 {
				sent := make(map[history.ClusterShardID]bool, len(shardToAck))
				logged := make(map[history.ClusterShardID]bool, len(shardToAck))
				numRemaining := len(shardToAck)
				backoff := 10 * time.Millisecond
				for numRemaining > 0 {
					select {
					case <-shutdownChan.Channel():
						return nil
					default:
					}
					progress := false
					for srcShard, originalAck := range shardToAck {
						if sent[srcShard] {
							continue
						}
						routedAck := &RoutedAck{
							TargetShard: s.targetShardID,
							Req: &adminservice.StreamWorkflowReplicationMessagesRequest{
								Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
									SyncReplicationState: &replicationv1.SyncReplicationState{
										InclusiveLowWatermark:     originalAck,
										InclusiveLowWatermarkTime: attr.SyncReplicationState.InclusiveLowWatermarkTime,
									},
								},
							},
						}

						s.logger.Info("Sender forwarding ACK to source shard", tag.NewStringTag("sourceShard", ClusterShardIDtoString(srcShard)), tag.NewInt64("ack", originalAck))

						if s.shardManager.DeliverAckToShardOwner(srcShard, routedAck, s.proxy, shutdownChan, s.logger, originalAck, true) {
							sent[srcShard] = true
							numRemaining--
							progress = true
							// record last ack per source shard after forwarding
							s.mu.Lock()
							s.prevAckBySource[srcShard] = originalAck
							s.mu.Unlock()
						} else if !logged[srcShard] {
							s.logger.Warn("No local ack channel for source shard; retrying until available", tag.NewStringTag("shard", ClusterShardIDtoString(srcShard)))
							logged[srcShard] = true
						}
					}
					if !progress {
						time.Sleep(backoff)
						if backoff < time.Second {
							backoff *= 2
						}
					} else if backoff > 10*time.Millisecond {
						backoff = 10 * time.Millisecond
					}
				}

				// TODO: ack to idle shards using prevAckBySource

			} else {
				// No new shards to ACK: send previous ack levels per source shard (if known)
				s.mu.Lock()
				pendingPrev := make(map[history.ClusterShardID]int64, len(s.prevAckBySource))
				for srcShard, prev := range s.prevAckBySource {
					pendingPrev[srcShard] = prev
				}
				s.mu.Unlock()

				sent := make(map[history.ClusterShardID]bool, len(pendingPrev))
				logged := make(map[history.ClusterShardID]bool, len(pendingPrev))
				numRemaining := len(pendingPrev)
				backoff := 10 * time.Millisecond
				for numRemaining > 0 {
					select {
					case <-shutdownChan.Channel():
						return nil
					default:
					}
					progress := false
					for srcShard, prev := range pendingPrev {
						if sent[srcShard] {
							continue
						}
						routedAck := &RoutedAck{
							TargetShard: s.targetShardID,
							Req: &adminservice.StreamWorkflowReplicationMessagesRequest{
								Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
									SyncReplicationState: &replicationv1.SyncReplicationState{
										InclusiveLowWatermark:     prev,
										InclusiveLowWatermarkTime: attr.SyncReplicationState.InclusiveLowWatermarkTime,
									},
								},
							},
						}
						// Log fallback ACK for this source shard
						s.logger.Info("Sender forwarding fallback ACK to source shard", tag.NewStringTag("sourceShard", ClusterShardIDtoString(srcShard)), tag.NewInt64("ack", prev))
						if s.shardManager.DeliverAckToShardOwner(srcShard, routedAck, s.proxy, shutdownChan, s.logger, prev, true) {
							sent[srcShard] = true
							numRemaining--
							progress = true
						} else if !logged[srcShard] {
							s.logger.Warn("No local ack channel for source shard; retrying until available", tag.NewStringTag("shard", ClusterShardIDtoString(srcShard)))
							logged[srcShard] = true
						}
					}
					if !progress {
						time.Sleep(backoff)
						if backoff < time.Second {
							backoff *= 2
						}
					} else if backoff > 10*time.Millisecond {
						backoff = 10 * time.Millisecond
					}
				}
			}

			// Only after forwarding ACKs, discard the entries from the ring buffer
			if pendingDiscard > 0 {
				s.mu.Lock()
				s.idRing.Discard(pendingDiscard)
				s.mu.Unlock()
			}

			// Update debug snapshot after ack processing
			s.streamTracker.UpdateStreamSenderDebug(s.streamID, s.buildSenderDebugSnapshot(20))
		}
	}
	return nil
}

// sendReplicationMessages sends replication messages read from sendMsgChan to
// the remote side. This is a placeholder implementation.
func (s *proxyStreamSender) sendReplicationMessages(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	shutdownChan channel.ShutdownOnce,
) error {
	defer func() {
		s.logger.Info("Shutdown sendMsgChan loop.")
		shutdownChan.Shutdown()
	}()
	for !shutdownChan.IsShutdown() {
		if s.sendMsgChan == nil {
			return nil
		}
		select {
		case routed, ok := <-s.sendMsgChan:
			if !ok {
				return nil
			}
			resp := routed.Resp
			if m, ok := resp.Attributes.(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages); ok && m.Messages != nil {
				// rewrite task ids
				s.mu.Lock()
				var originalIDs []int64
				var proxyIDs []int64
				// capture original exclusive high watermark before rewriting
				originalHigh := m.Messages.ExclusiveHighWatermark
				for _, t := range m.Messages.ReplicationTasks {
					// allocate proxy task id
					s.nextProxyTaskID++
					proxyID := s.nextProxyTaskID
					// remember original
					original := t.SourceTaskId
					originalIDs = append(originalIDs, original)
					s.idRing.Append(proxyID, routed.SourceShard, original)
					// rewrite id
					t.SourceTaskId = proxyID
					if t.RawTaskInfo != nil {
						t.RawTaskInfo.TaskId = proxyID
					}
					proxyIDs = append(proxyIDs, proxyID)
				}
				s.mu.Unlock()
				// Log mapping from original -> proxy IDs
				s.logger.Info(fmt.Sprintf("Sender forwarding ReplicationTasks from shard %s: original=%v proxy=%v", ClusterShardIDtoString(routed.SourceShard), originalIDs, proxyIDs))

				// Ensure exclusive high watermark is in proxy task ID space
				if len(m.Messages.ReplicationTasks) > 0 {
					m.Messages.ExclusiveHighWatermark = m.Messages.ReplicationTasks[len(m.Messages.ReplicationTasks)-1].SourceTaskId + 1
				} else {
					// No tasks in this batch: allocate a synthetic proxy task id mapping
					s.mu.Lock()
					s.nextProxyTaskID++
					proxyHigh := s.nextProxyTaskID
					s.idRing.Append(proxyHigh, routed.SourceShard, originalHigh)
					m.Messages.ExclusiveHighWatermark = proxyHigh
					s.mu.Unlock()
				}
				// track sent tasks ids and high watermark
				ids := make([]int64, 0, len(m.Messages.ReplicationTasks))
				for _, t := range m.Messages.ReplicationTasks {
					ids = append(ids, t.SourceTaskId)
				}
				s.streamTracker.UpdateStreamLastTaskIDs(s.streamID, ids)
				s.streamTracker.UpdateStreamReplicationMessages(s.streamID, m.Messages.ExclusiveHighWatermark)
				s.streamTracker.UpdateStreamSenderDebug(s.streamID, s.buildSenderDebugSnapshot(20))
				s.streamTracker.UpdateStream(s.streamID)
			}
			if err := targetStreamServer.Send(resp); err != nil {
				return err
			}
		case <-shutdownChan.Channel():
			return nil
		}
	}
	return nil
}

// proxyStreamReceiver receives replication messages from a local/remote server and
// produces ACKs destined for the original sender.
type proxyStreamReceiver struct {
	logger          log.Logger
	shardManager    ShardManager
	proxy           *Proxy
	adminClient     adminservice.AdminServiceClient
	localShardCount int32
	targetShardID   history.ClusterShardID
	sourceShardID   history.ClusterShardID
	directionLabel  string
	ackChan         chan RoutedAck
	// ack aggregation across target shards
	ackByTarget map[history.ClusterShardID]int64
	lastSentMin int64
	// lastExclusiveHighOriginal tracks last exclusive high watermark seen from source (original id space)
	lastExclusiveHighOriginal int64
	streamID                  string
	streamTracker             *StreamTracker
}

// buildReceiverDebugSnapshot builds receiver ACK aggregation state for debugging
func (r *proxyStreamReceiver) buildReceiverDebugSnapshot() *ReceiverDebugInfo {
	info := &ReceiverDebugInfo{
		AckByTarget: make(map[string]int64),
	}
	for k, v := range r.ackByTarget {
		info.AckByTarget[ClusterShardIDtoString(k)] = v
	}
	info.LastAggregatedMin = r.lastSentMin
	info.LastExclusiveHighOriginal = r.lastExclusiveHighOriginal
	return info
}

func (r *proxyStreamReceiver) Run(
	shutdownChan channel.ShutdownOnce,
) {
	// Terminate any previous local receiver for this shard
	r.proxy.TerminatePreviousLocalReceiver(r.sourceShardID)

	r.logger = log.With(r.logger,
		tag.NewStringTag("client", ClusterShardIDtoString(r.targetShardID)),
		tag.NewStringTag("server", ClusterShardIDtoString(r.sourceShardID)),
		tag.NewStringTag("stream-source-shard", ClusterShardIDtoString(r.sourceShardID)),
		tag.NewStringTag("stream-target-shard", ClusterShardIDtoString(r.targetShardID)),
		tag.NewStringTag("role", "receiver"),
	)
	r.logger.Info("proxyStreamReceiver Run")
	defer r.logger.Info("proxyStreamReceiver Run finished")

	// Build metadata for local server stream
	md := metadata.New(map[string]string{})
	md.Set(history.MetadataKeyClientClusterID, fmt.Sprintf("%d", r.targetShardID.ClusterID))
	md.Set(history.MetadataKeyClientShardID, fmt.Sprintf("%d", r.targetShardID.ShardID))
	md.Set(history.MetadataKeyServerClusterID, fmt.Sprintf("%d", r.sourceShardID.ClusterID))
	md.Set(history.MetadataKeyServerShardID, fmt.Sprintf("%d", r.sourceShardID.ShardID))

	outgoingContext := metadata.NewOutgoingContext(context.Background(), md)
	outgoingContext, cancel := context.WithCancel(outgoingContext)
	defer cancel()

	// Open stream receiver -> local server's stream sender for clientShardID
	var sourceStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient
	var err error
	sourceStreamClient, err = r.adminClient.StreamWorkflowReplicationMessages(outgoingContext)
	if err != nil {
		r.logger.Error("adminClient.StreamWorkflowReplicationMessages error", tag.Error(err))
		return
	}

	// Setup ack channel and cancel func bookkeeping
	r.ackChan = make(chan RoutedAck, 100)
	r.proxy.SetLocalAckChan(r.sourceShardID, r.ackChan)
	r.proxy.SetLocalReceiverCancelFunc(r.sourceShardID, cancel)
	defer func() {
		r.proxy.RemoveLocalAckChan(r.sourceShardID)
		r.proxy.RemoveLocalReceiverCancelFunc(r.sourceShardID)
	}()

	// init aggregation state
	r.ackByTarget = make(map[history.ClusterShardID]int64)
	r.lastSentMin = 0

	// Register a new local stream for tracking (short id, include role)
	r.streamID = BuildReceiverStreamID(r.sourceShardID, r.targetShardID)
	r.logger = log.With(r.logger, tag.NewStringTag("streamID", r.streamID))
	r.streamTracker = GetGlobalStreamTracker()
	r.streamTracker.RegisterStream(
		r.streamID,
		"StreamWorkflowReplicationMessages",
		r.directionLabel,
		ClusterShardIDtoString(r.sourceShardID),
		ClusterShardIDtoString(r.targetShardID),
		StreamRoleReceiver,
	)
	defer r.streamTracker.UnregisterStream(r.streamID)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer func() {
			shutdownChan.Shutdown()
			wg.Done()
		}()
		_ = r.recvReplicationMessages(sourceStreamClient, shutdownChan)
	}()

	go func() {
		defer func() {
			shutdownChan.Shutdown()
			_ = sourceStreamClient.CloseSend()
			wg.Done()
		}()
		_ = r.sendAck(sourceStreamClient, shutdownChan)
	}()

	wg.Wait()
}

// recvReplicationMessages receives from local server and routes to target shard owners.
func (r *proxyStreamReceiver) recvReplicationMessages(
	sourceStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	shutdownChan channel.ShutdownOnce,
) error {
	for !shutdownChan.IsShutdown() {
		resp, err := sourceStreamClient.Recv()
		if err == io.EOF {
			r.logger.Info("sourceStreamClient.Recv encountered EOF", tag.Error(err))
			return nil
		}
		if err != nil {
			r.logger.Error("sourceStreamClient.Recv encountered error", tag.Error(err))
			return err
		}

		if attr, ok := resp.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages); ok && attr.Messages != nil {
			// Group by recalculated target shard using namespace/workflow hash
			tasksByTargetShard := make(map[history.ClusterShardID][]*replicationv1.ReplicationTask)
			ids := make([]int64, 0, len(attr.Messages.ReplicationTasks))
			for _, task := range attr.Messages.ReplicationTasks {
				if task.RawTaskInfo != nil && task.RawTaskInfo.NamespaceId != "" && task.RawTaskInfo.WorkflowId != "" {
					targetShard := servercommon.WorkflowIDToHistoryShard(task.RawTaskInfo.NamespaceId, task.RawTaskInfo.WorkflowId, r.localShardCount)
					targetClusterShard := history.ClusterShardID{ClusterID: r.targetShardID.ClusterID, ShardID: targetShard}
					tasksByTargetShard[targetClusterShard] = append(tasksByTargetShard[targetClusterShard], task)
					ids = append(ids, task.SourceTaskId)
				}
			}

			// Log every replication task id received at receiver
			r.logger.Info(fmt.Sprintf("Receiver received ReplicationTasks: exclusive_high=%d ids=%v", attr.Messages.ExclusiveHighWatermark, ids))

			// record last source exclusive high watermark (original id space)
			r.lastExclusiveHighOriginal = attr.Messages.ExclusiveHighWatermark

			// update tracker for incoming messages
			if r.streamTracker != nil && r.streamID != "" {
				r.streamTracker.UpdateStreamLastTaskIDs(r.streamID, ids)
				r.streamTracker.UpdateStreamReplicationMessages(r.streamID, attr.Messages.ExclusiveHighWatermark)
				r.streamTracker.UpdateStreamReceiverDebug(r.streamID, r.buildReceiverDebugSnapshot())
				r.streamTracker.UpdateStream(r.streamID)
			}

			// If replication tasks are empty, still log the empty batch and send watermark
			if len(attr.Messages.ReplicationTasks) == 0 {
				r.logger.Info("Receiver received empty replication batch", tag.NewInt64("exclusive_high", attr.Messages.ExclusiveHighWatermark))
				msg := RoutedMessage{
					SourceShard: r.sourceShardID,
					Resp: &adminservice.StreamWorkflowReplicationMessagesResponse{
						Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
							Messages: &replicationv1.WorkflowReplicationMessages{
								ExclusiveHighWatermark: attr.Messages.ExclusiveHighWatermark,
								Priority:               attr.Messages.Priority,
							},
						},
					},
				}
				localShardsToSend := r.proxy.GetRemoteSendChansByCluster(r.targetShardID.ClusterID)
				r.logger.Info("Going to broadcast high watermark to local shards", tag.NewStringTag("localShardsToSend", fmt.Sprintf("%v", localShardsToSend)))
				for targetShardID, sendChan := range localShardsToSend {
					r.logger.Info("Sending high watermark to target shard", tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)), tag.NewInt64("exclusive_high", attr.Messages.ExclusiveHighWatermark))
					sendChan <- msg
				}
				// send to all remote shards on other nodes as well
				remoteShards, err := r.shardManager.GetRemoteShardsForPeer("")
				if err != nil {
					r.logger.Error("Failed to get remote shards", tag.Error(err))
					return err
				}
				r.logger.Info("Going to broadcast high watermark to remote shards", tag.NewStringTag("remoteShards", fmt.Sprintf("%v", remoteShards)))
				for _, shards := range remoteShards {
					for _, shard := range shards.Shards {
						if shard.ID.ClusterID != r.targetShardID.ClusterID {
							continue
						}
						if !r.shardManager.DeliverMessagesToShardOwner(shard.ID, &msg, r.proxy, shutdownChan, r.logger) {
							r.logger.Warn("Failed to send ReplicationTasks to remote shard", tag.NewStringTag("shard", ClusterShardIDtoString(shard.ID)))
						}
					}
				}
			}

			// Retry across the whole target set until all sends succeed (or shutdown)
			sentByTarget := make(map[history.ClusterShardID]bool, len(tasksByTargetShard))
			loggedByTarget := make(map[history.ClusterShardID]bool, len(tasksByTargetShard))
			for targetShardID := range tasksByTargetShard {
				sentByTarget[targetShardID] = false
			}
			numRemaining := len(tasksByTargetShard)
			backoff := 10 * time.Millisecond
			for numRemaining > 0 {
				select {
				case <-shutdownChan.Channel():
					return nil
				default:
				}
				progress := false
				for targetShardID, tasks := range tasksByTargetShard {
					if sentByTarget[targetShardID] {
						continue
					}
					msg := RoutedMessage{
						SourceShard: r.sourceShardID,
						Resp: &adminservice.StreamWorkflowReplicationMessagesResponse{
							Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
								Messages: &replicationv1.WorkflowReplicationMessages{
									ReplicationTasks:       tasks,
									ExclusiveHighWatermark: tasks[len(tasks)-1].RawTaskInfo.TaskId + 1,
									Priority:               attr.Messages.Priority,
								},
							},
						},
					}
					if r.shardManager.DeliverMessagesToShardOwner(targetShardID, &msg, r.proxy, shutdownChan, r.logger) {
						sentByTarget[targetShardID] = true
						numRemaining--
						progress = true
					} else {
						if !loggedByTarget[targetShardID] {
							r.logger.Warn("No send channel found for target shard; retrying until available", tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)))
							loggedByTarget[targetShardID] = true
						}
					}
				}
				if !progress {
					time.Sleep(backoff)
					if backoff < time.Second {
						backoff *= 2
					}
				} else if backoff > 10*time.Millisecond {
					backoff = 10 * time.Millisecond
				}
			}
		}
	}
	return nil
}

// sendAck forwards ACKs from local ack channel upstream to the local server.
func (r *proxyStreamReceiver) sendAck(
	sourceStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	shutdownChan channel.ShutdownOnce,
) error {
	for !shutdownChan.IsShutdown() {
		select {
		case routed := <-r.ackChan:
			// Update per-target watermark
			if attr, ok := routed.Req.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState); ok && attr.SyncReplicationState != nil {
				r.ackByTarget[routed.TargetShard] = attr.SyncReplicationState.InclusiveLowWatermark
				// Compute minimal watermark across targets
				min := int64(0)
				first := true
				for _, wm := range r.ackByTarget {
					if first || wm < min {
						min = wm
						first = false
					}
				}
				if !first && min >= r.lastSentMin {
					// Clamp ACK to last known exclusive high watermark from source
					if r.lastExclusiveHighOriginal > 0 && min > r.lastExclusiveHighOriginal {
						r.logger.Warn("Aggregated ACK exceeds last source high watermark; clamping",
							tag.NewInt64("ack_min", min),
							tag.NewInt64("source_exclusive_high", r.lastExclusiveHighOriginal))
						min = r.lastExclusiveHighOriginal
					}
					// Send aggregated minimal ack upstream
					aggregated := &adminservice.StreamWorkflowReplicationMessagesRequest{
						Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
							SyncReplicationState: &replicationv1.SyncReplicationState{
								InclusiveLowWatermark: min,
							},
						},
					}
					r.logger.Info("Receiver sending aggregated ACK upstream", tag.NewInt64("inclusive_low", min))
					if err := sourceStreamClient.Send(aggregated); err != nil {
						if err != io.EOF {
							r.logger.Error("sourceStreamClient.Send encountered error", tag.Error(err))
						} else {
							r.logger.Info("sourceStreamClient.Send encountered EOF", tag.Error(err))
						}
						return err
					}
					// Track sync watermark for receiver stream
					if r.streamTracker != nil && r.streamID != "" {
						r.streamTracker.UpdateStreamSyncReplicationState(r.streamID, min, nil)
						r.streamTracker.UpdateStream(r.streamID)
						// Update receiver debug snapshot when we send an aggregated ACK
						r.streamTracker.UpdateStreamReceiverDebug(r.streamID, r.buildReceiverDebugSnapshot())
					}
					r.lastSentMin = min
				}
			}
		case <-shutdownChan.Channel():
			return nil
		}
	}
	return nil
}
