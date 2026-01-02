package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"sync"
	"time"

	"go.temporal.io/server/api/adminservice/v1"
	replicationv1 "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

// RoutedAck wraps an ACK with the target shard it originated from
type RoutedAck struct {
	TargetShard history.ClusterShardID
	Req         *adminservice.StreamWorkflowReplicationMessagesRequest
}

// RoutedMessage wraps a replication response with originating client shard info
type RoutedMessage struct {
	SourceShard history.ClusterShardID
	Resp        *adminservice.StreamWorkflowReplicationMessagesResponse
}

// intraProxyManager maintains long-lived intra-proxy streams to peer proxies and
// provides simple send helpers (e.g., forwarding ACKs).
type intraProxyManager struct {
	logger       log.Logger
	streamsMu    sync.RWMutex
	shardManager ShardManager
	notifyCh     chan struct{}
	// Group state by remote peer for unified lifecycle ops
	peers map[string]*peerState
}

type peerState struct {
	conn         *grpc.ClientConn
	receivers    map[peerStreamKey]*intraProxyStreamReceiver
	senders      map[peerStreamKey]*intraProxyStreamSender
	recvShutdown map[peerStreamKey]channel.ShutdownOnce
}

type peerStreamKey struct {
	targetShard history.ClusterShardID
	sourceShard history.ClusterShardID
}

func newIntraProxyManager(logger log.Logger, shardManager ShardManager) *intraProxyManager {
	return &intraProxyManager{
		logger:       logger,
		shardManager: shardManager,
		peers:        make(map[string]*peerState),
		notifyCh:     make(chan struct{}),
	}
}

// intraProxyStreamSender registers server stream and forwards upstream ACKs to shard owners (local or remote).
// Replication messages are sent by intraProxyManager.sendMessages using the registered server stream.
type intraProxyStreamSender struct {
	logger             log.Logger
	shardManager       ShardManager
	peerNodeName       string
	targetShardID      history.ClusterShardID
	sourceShardID      history.ClusterShardID
	streamID           string
	sourceStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer
}

func (s *intraProxyStreamSender) Run(
	sourceStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	shutdownChan channel.ShutdownOnce,
) error {
	s.streamID = BuildIntraProxySenderStreamID(s.peerNodeName, s.sourceShardID, s.targetShardID)
	s.logger = log.With(s.logger, tag.NewStringTag("streamID", s.streamID))

	s.logger.Info("intraProxyStreamSender Run")
	defer s.logger.Info("intraProxyStreamSender Run finished")

	// Register server-side intra-proxy stream in tracker
	st := GetGlobalStreamTracker()
	st.RegisterStream(s.streamID, "StreamWorkflowReplicationMessages", "intra-proxy", ClusterShardIDtoString(s.sourceShardID), ClusterShardIDtoString(s.targetShardID), StreamRoleForwarder)
	defer st.UnregisterStream(s.streamID)

	s.sourceStreamServer = sourceStreamServer

	// register this sender so sendMessages can use it
	s.shardManager.GetIntraProxyManager().RegisterSender(s.peerNodeName, s.targetShardID, s.sourceShardID, s)
	defer s.shardManager.GetIntraProxyManager().UnregisterSender(s.peerNodeName, s.targetShardID, s.sourceShardID)

	// Send pending watermarks to late-registering shards
	// When a sender is registered, check if there's an active receiver for the source shard
	// that has a pending watermark, and send it immediately to the peer
	if receiver, ok := s.shardManager.GetActiveReceiver(s.sourceShardID); ok {
		if lastWatermark := receiver.GetLastWatermark(); lastWatermark != nil && lastWatermark.ExclusiveHighWatermark > 0 {
			s.logger.Debug("Sending pending watermark to peer on sender registration",
				tag.NewInt64("exclusive_high", lastWatermark.ExclusiveHighWatermark),
				tag.NewStringTag("peer", s.peerNodeName))
			resp := &adminservice.StreamWorkflowReplicationMessagesResponse{
				Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
					Messages: &replicationv1.WorkflowReplicationMessages{
						ExclusiveHighWatermark: lastWatermark.ExclusiveHighWatermark,
						Priority:               lastWatermark.Priority,
					},
				},
			}
			if err := s.sendReplicationMessages(resp); err != nil {
				s.logger.Warn("Failed to send pending watermark to peer on sender registration", tag.Error(err))
			}
		}
	}

	// recv ACKs from peer and route to original source shard owner
	return s.recvAck(shutdownChan)
}

// recvAck reads ACKs from the peer and routes them to the source shard owner.
func (s *intraProxyStreamSender) recvAck(shutdownChan channel.ShutdownOnce) error {
	s.logger.Debug("intraProxyStreamSender recvAck")
	defer func() {
		s.logger.Debug("intraProxyStreamSender recvAck finished")
		shutdownChan.Shutdown()
	}()

	for !shutdownChan.IsShutdown() {
		req, err := s.sourceStreamServer.Recv()
		if err == io.EOF {
			s.logger.Debug("intraProxyStreamSender recvAck encountered EOF")
			return nil
		}
		if err != nil {
			s.logger.Error("intraProxyStreamSender recvAck encountered error", tag.Error(err))
			return err
		}
		if attr, ok := req.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState); ok && attr.SyncReplicationState != nil {
			ack := attr.SyncReplicationState.InclusiveLowWatermark

			s.logger.Debug("Sender received upstream ACK", tag.NewInt64("inclusive_low", ack))

			// Update server-side intra-proxy stream tracker with sync watermark
			st := GetGlobalStreamTracker()
			st.UpdateStreamSyncReplicationState(s.streamID, ack, nil)
			st.UpdateStream(s.streamID)

			routedAck := &RoutedAck{
				TargetShard: s.targetShardID,
				Req: &adminservice.StreamWorkflowReplicationMessagesRequest{
					Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
						SyncReplicationState: &replicationv1.SyncReplicationState{InclusiveLowWatermark: ack},
					},
				},
			}

			s.logger.Debug("Sender forwarding ACK to source shard", tag.NewStringTag("sourceShard", ClusterShardIDtoString(s.sourceShardID)), tag.NewInt64("ack", ack))
			// FIXME: should retry. If not succeed, return and shutdown the stream
			sent := s.shardManager.DeliverAckToShardOwner(s.sourceShardID, routedAck, shutdownChan, s.logger, ack, false)
			if !sent {
				s.logger.Error("Sender failed to forward ACK to source shard", tag.NewStringTag("sourceShard", ClusterShardIDtoString(s.sourceShardID)), tag.NewInt64("ack", ack))
				return fmt.Errorf("failed to forward ACK to source shard")
			}
		}
	}
	return nil
}

// sendReplicationMessages sends replication messages to the peer via the server stream.
func (s *intraProxyStreamSender) sendReplicationMessages(resp *adminservice.StreamWorkflowReplicationMessagesResponse) error {
	s.logger.Info("intraProxyStreamSender sendReplicationMessages started")
	defer s.logger.Info("intraProxyStreamSender sendReplicationMessages finished")

	// Update server-side intra-proxy tracker for outgoing messages
	if msgs, ok := resp.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages); ok && msgs.Messages != nil {
		st := GetGlobalStreamTracker()
		ids := make([]int64, 0, len(msgs.Messages.ReplicationTasks))
		for _, t := range msgs.Messages.ReplicationTasks {
			ids = append(ids, t.SourceTaskId)
		}
		st.UpdateStreamLastTaskIDs(s.streamID, ids)
		st.UpdateStreamReplicationMessages(s.streamID, msgs.Messages.ExclusiveHighWatermark)
		st.UpdateStream(s.streamID)
	}
	if err := s.sourceStreamServer.Send(resp); err != nil {
		return err
	}
	return nil
}

// intraProxyStreamReceiver ensures a client stream to peer exists and sends aggregated ACKs upstream.
type intraProxyStreamReceiver struct {
	logger        log.Logger
	shardManager  ShardManager
	intraMgr      *intraProxyManager
	peerNodeName  string
	targetShardID history.ClusterShardID
	sourceShardID history.ClusterShardID
	streamClient  adminservice.AdminService_StreamWorkflowReplicationMessagesClient
	streamID      string
	shutdown      channel.ShutdownOnce
	cancel        context.CancelFunc
	// lastWatermark tracks the last watermark received from source shard for late-registering target shards
	lastWatermarkMu sync.RWMutex
	lastWatermark   *replicationv1.WorkflowReplicationMessages
}

// Run opens the client stream with metadata, registers tracking, and starts receiver goroutines.
func (r *intraProxyStreamReceiver) Run(ctx context.Context, shardManager ShardManager, conn *grpc.ClientConn) error {
	r.streamID = BuildIntraProxyReceiverStreamID(r.peerNodeName, r.sourceShardID, r.targetShardID)
	r.logger = log.With(r.logger, tag.NewStringTag("streamID", r.streamID))

	r.logger.Info("intraProxyStreamReceiver Run")
	// Build metadata according to receiver pattern: client=targetShard, server=sourceShard
	md := metadata.New(map[string]string{})
	md.Set(history.MetadataKeyClientClusterID, fmt.Sprintf("%d", r.targetShardID.ClusterID))
	md.Set(history.MetadataKeyClientShardID, fmt.Sprintf("%d", r.targetShardID.ShardID))
	md.Set(history.MetadataKeyServerClusterID, fmt.Sprintf("%d", r.sourceShardID.ClusterID))
	md.Set(history.MetadataKeyServerShardID, fmt.Sprintf("%d", r.sourceShardID.ShardID))
	ctx = metadata.NewOutgoingContext(ctx, md)
	ctx = common.WithIntraProxyHeaders(ctx, map[string]string{
		common.IntraProxyOriginProxyIDHeader: shardManager.GetShardInfo().NodeName,
	})

	// Ensure we can cancel Recv() by canceling the context when tearing down
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	client := adminservice.NewAdminServiceClient(conn)
	streamClient, err := client.StreamWorkflowReplicationMessages(ctx)
	if err != nil {
		if r.cancel != nil {
			r.cancel()
		}
		return err
	}
	r.streamClient = streamClient

	r.shardManager.RegisterActiveReceiver(r.sourceShardID, r)
	defer r.shardManager.UnregisterActiveReceiver(r.sourceShardID)

	// Register client-side intra-proxy stream in tracker
	st := GetGlobalStreamTracker()
	st.RegisterStream(r.streamID, "StreamWorkflowReplicationMessages", "intra-proxy", ClusterShardIDtoString(r.sourceShardID), ClusterShardIDtoString(r.targetShardID), StreamRoleForwarder)
	defer st.UnregisterStream(r.streamID)

	// Start replication receiver loop
	return r.recvReplicationMessages()
}

// recvReplicationMessages receives replication messages and forwards to local shard owner.
func (r *intraProxyStreamReceiver) recvReplicationMessages() error {
	r.logger.Debug("intraProxyStreamReceiver recvReplicationMessages started")
	defer r.logger.Debug("intraProxyStreamReceiver recvReplicationMessages finished")

	shutdown := r.shutdown
	defer shutdown.Shutdown()
	backoff := 10 * time.Millisecond
	for !shutdown.IsShutdown() {
		resp, err := r.streamClient.Recv()
		if err == io.EOF {
			r.logger.Debug("recvReplicationMessages encountered EOF")
			return nil
		}
		if err != nil {
			r.logger.Error("intra-proxy stream Recv error", tag.Error(err))
			return err
		}
		if msgs, ok := resp.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages); ok && msgs.Messages != nil {
			// Capture watermark value immediately to avoid data race with sender
			exclusiveHighWatermark := msgs.Messages.ExclusiveHighWatermark
			priority := msgs.Messages.Priority

			// Update client-side intra-proxy tracker for received messages
			st := GetGlobalStreamTracker()
			ids := make([]int64, 0, len(msgs.Messages.ReplicationTasks))
			for _, t := range msgs.Messages.ReplicationTasks {
				ids = append(ids, t.SourceTaskId)
			}
			st.UpdateStreamLastTaskIDs(r.streamID, ids)
			st.UpdateStreamReplicationMessages(r.streamID, exclusiveHighWatermark)
			st.UpdateStream(r.streamID)

			// Track last watermark for late-registering shards
			r.lastWatermarkMu.Lock()
			r.lastWatermark = &replicationv1.WorkflowReplicationMessages{
				ExclusiveHighWatermark: exclusiveHighWatermark,
				Priority:               priority,
			}
			r.lastWatermarkMu.Unlock()

			r.logger.Debug(fmt.Sprintf("Receiver received ReplicationTasks: exclusive_high=%d ids=%v", exclusiveHighWatermark, ids))

			msg := RoutedMessage{SourceShard: r.sourceShardID, Resp: resp}
			sent := false
			logged := false
			for !sent {
				if ch, ok := r.shardManager.GetRemoteSendChan(r.targetShardID); ok {
					func() {
						defer func() {
							if panicErr := recover(); panicErr != nil {
								r.logger.Warn("Failed to send to local target shard (channel closed)",
									tag.NewStringTag("targetShard", ClusterShardIDtoString(r.targetShardID)))
							}
						}()
						select {
						case ch <- msg:
							sent = true
							r.logger.Debug("Receiver sent ReplicationTasks to local target shard", tag.NewStringTag("targetShard", ClusterShardIDtoString(r.targetShardID)), tag.NewInt64("exclusive_high", exclusiveHighWatermark))
						case <-shutdown.Channel():
							// Will be handled outside the func
						}
					}()
					if shutdown.IsShutdown() {
						return nil
					}
				} else {
					if !logged {
						r.logger.Warn("No local send channel yet for target shard; waiting",
							tag.NewStringTag("targetShard", ClusterShardIDtoString(r.targetShardID)))
						logged = true
					}
					time.Sleep(backoff)
					if backoff < time.Second {
						backoff *= 2
					}
				}
			}
			backoff = 10 * time.Millisecond
		}
	}
	return nil
}

// sendAck sends an ACK upstream via the client stream and updates tracker.
func (r *intraProxyStreamReceiver) sendAck(req *adminservice.StreamWorkflowReplicationMessagesRequest) error {
	r.logger.Debug("intraProxyStreamReceiver sendAck started")
	defer r.logger.Debug("intraProxyStreamReceiver sendAck finished")

	if err := r.streamClient.Send(req); err != nil {
		return err
	}
	if attr, ok := req.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState); ok && attr.SyncReplicationState != nil {
		st := GetGlobalStreamTracker()
		st.UpdateStreamSyncReplicationState(r.streamID, attr.SyncReplicationState.InclusiveLowWatermark, nil)
		st.UpdateStream(r.streamID)
	}
	return nil
}

// GetTargetShardID returns the target shard ID for this receiver
func (r *intraProxyStreamReceiver) GetTargetShardID() history.ClusterShardID {
	return r.targetShardID
}

// GetSourceShardID returns the source shard ID for this receiver
func (r *intraProxyStreamReceiver) GetSourceShardID() history.ClusterShardID {
	return r.sourceShardID
}

// GetLastWatermark returns the last watermark received from the source shard
func (r *intraProxyStreamReceiver) GetLastWatermark() *replicationv1.WorkflowReplicationMessages {
	r.lastWatermarkMu.RLock()
	defer r.lastWatermarkMu.RUnlock()
	return r.lastWatermark
}

// NotifyNewTargetShard notifies the receiver about a newly registered target shard
func (r *intraProxyStreamReceiver) NotifyNewTargetShard(targetShardID history.ClusterShardID) {
	r.sendPendingWatermarkToShard(targetShardID)
}

// sendPendingWatermarkToShard sends the last known watermark to a newly registered target shard
// This ensures late-registering shards receive watermarks that were sent before they registered
func (r *intraProxyStreamReceiver) sendPendingWatermarkToShard(targetShardID history.ClusterShardID) {
	r.lastWatermarkMu.RLock()
	lastWatermark := r.lastWatermark
	r.lastWatermarkMu.RUnlock()

	if lastWatermark == nil || lastWatermark.ExclusiveHighWatermark == 0 {
		// No pending watermark to send
		return
	}

	r.logger.Debug("Sending pending watermark to newly registered shard",
		tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)),
		tag.NewInt64("exclusive_high", lastWatermark.ExclusiveHighWatermark))

	msg := RoutedMessage{
		SourceShard: r.sourceShardID,
		Resp: &adminservice.StreamWorkflowReplicationMessagesResponse{
			Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
				Messages: &replicationv1.WorkflowReplicationMessages{
					ExclusiveHighWatermark: lastWatermark.ExclusiveHighWatermark,
					Priority:               lastWatermark.Priority,
				},
			},
		},
	}

	// Try to send to local shard first
	if sendChan, exists := r.shardManager.GetRemoteSendChan(targetShardID); exists {
		clonedResp := proto.Clone(msg.Resp).(*adminservice.StreamWorkflowReplicationMessagesResponse)
		clonedMsg := RoutedMessage{
			SourceShard: msg.SourceShard,
			Resp:        clonedResp,
		}
		select {
		case sendChan <- clonedMsg:
			r.logger.Debug("Sent pending watermark to local shard",
				tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)))
		default:
			r.logger.Warn("Failed to send pending watermark to local shard (channel full)",
				tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)))
		}
		return
	}

	// If not local, try to send to remote shard
	if r.shardManager != nil {
		shutdownChan := channel.NewShutdownOnce()
		clonedResp := proto.Clone(msg.Resp).(*adminservice.StreamWorkflowReplicationMessagesResponse)
		clonedMsg := RoutedMessage{
			SourceShard: msg.SourceShard,
			Resp:        clonedResp,
		}
		if r.shardManager.DeliverMessagesToShardOwner(targetShardID, &clonedMsg, shutdownChan, r.logger) {
			r.logger.Debug("Sent pending watermark to remote shard",
				tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)))
		} else {
			r.logger.Warn("Failed to send pending watermark to remote shard",
				tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShardID)))
		}
	}
}

func (m *intraProxyManager) RegisterSender(
	peerNodeName string,
	targetShard history.ClusterShardID,
	sourceShard history.ClusterShardID,
	sender *intraProxyStreamSender,
) {
	// Cross-cluster only
	if targetShard.ClusterID == sourceShard.ClusterID {
		return
	}
	key := peerStreamKey{targetShard: targetShard, sourceShard: sourceShard}
	m.logger.Info("RegisterSender", tag.NewStringTag("peerNodeName", peerNodeName), tag.NewStringTag("key", fmt.Sprintf("%v", key)), tag.NewStringTag("sender", sender.streamID))
	m.streamsMu.Lock()
	ps := m.peers[peerNodeName]
	if ps == nil {
		ps = &peerState{receivers: make(map[peerStreamKey]*intraProxyStreamReceiver), senders: make(map[peerStreamKey]*intraProxyStreamSender), recvShutdown: make(map[peerStreamKey]channel.ShutdownOnce)}
		m.peers[peerNodeName] = ps
	}
	if ps.senders == nil {
		ps.senders = make(map[peerStreamKey]*intraProxyStreamSender)
	}
	ps.senders[key] = sender
	m.streamsMu.Unlock()
}

func (m *intraProxyManager) UnregisterSender(
	peerNodeName string,
	targetShard history.ClusterShardID,
	sourceShard history.ClusterShardID,
) {
	key := peerStreamKey{targetShard: targetShard, sourceShard: sourceShard}
	m.logger.Info("UnregisterSender", tag.NewStringTag("peerNodeName", peerNodeName), tag.NewStringTag("key", fmt.Sprintf("%v", key)))
	m.streamsMu.Lock()
	if ps := m.peers[peerNodeName]; ps != nil && ps.senders != nil {
		delete(ps.senders, key)
	}
	m.streamsMu.Unlock()
}

// EnsureReceiverForPeerShard ensures a client stream and an ACK aggregator exist for the given peer/shard pair.
func (m *intraProxyManager) EnsureReceiverForPeerShard(peerNodeName string, targetShard history.ClusterShardID, sourceShard history.ClusterShardID) {
	logger := log.With(m.logger,
		tag.NewStringTag("peerNodeName", peerNodeName),
		tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShard)),
		tag.NewStringTag("sourceShard", ClusterShardIDtoString(sourceShard)))
	logger.Debug("EnsureReceiverForPeerShard")

	// Cross-cluster only
	if targetShard.ClusterID == sourceShard.ClusterID {
		return
	}
	// Do not create intra-proxy streams to self instance
	if peerNodeName == m.shardManager.GetNodeName() {
		return
	}
	// Require at least one shard to be local to this instance
	isLocalTargetShard := m.shardManager.IsLocalShard(targetShard)
	isLocalSourceShard := m.shardManager.IsLocalShard(sourceShard)
	if !isLocalTargetShard && !isLocalSourceShard {
		logger.Debug("EnsureReceiverForPeerShard skipping because neither shard is local", tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShard)), tag.NewStringTag("sourceShard", ClusterShardIDtoString(sourceShard)), tag.NewBoolTag("isLocalTargetShard", isLocalTargetShard), tag.NewBoolTag("isLocalSourceShard", isLocalSourceShard))
		return
	}
	// Consolidated path: ensure stream and background loops
	err := m.ensureStream(context.Background(), logger, peerNodeName, targetShard, sourceShard)
	if err != nil {
		logger.Error("failed to ensureStream", tag.Error(err))
	}
}

// ensurePeer ensures a per-peer state with a shared gRPC connection exists.
func (m *intraProxyManager) ensurePeer(
	ctx context.Context,
	peerNodeName string,
) (*peerState, error) {
	logger := log.With(m.logger, tag.NewStringTag("peerNodeName", peerNodeName))
	logger.Debug("ensurePeer started")
	defer logger.Debug("ensurePeer finished")

	m.streamsMu.RLock()
	if ps, ok := m.peers[peerNodeName]; ok && ps != nil && ps.conn != nil {
		m.streamsMu.RUnlock()
		logger.Debug("ensurePeer found existing peer with connection")
		return ps, nil
	}
	m.streamsMu.RUnlock()

	logger.Debug("ensurePeer creating new peer connection")

	// Build TLS from this proxy's outbound client TLS config if available
	tlsCfg := m.shardManager.GetIntraProxyTLSConfig()
	var parsedTLSCfg *tls.Config
	if tlsCfg.IsEnabled() {
		logger.Debug("ensurePeer TLS enabled, building TLS config")
		var err error
		parsedTLSCfg, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			logger.Error("ensurePeer failed to create TLS config", tag.Error(err))
			return nil, fmt.Errorf("config error when creating tls config: %w", err)
		}
	} else {
		logger.Debug("ensurePeer TLS disabled")
	}
	dialOpts := grpcutil.MakeDialOptions(parsedTLSCfg, metrics.GetGRPCClientMetrics("intra_proxy"))

	proxyAddresses, ok := m.shardManager.GetProxyAddress(peerNodeName)
	if !ok {
		logger.Error("ensurePeer proxy address not found")
		return nil, fmt.Errorf("proxy address not found")
	}
	logger.Debug("ensurePeer dialing peer", tag.NewStringTag("proxyAddresses", proxyAddresses))

	cc, err := grpc.NewClient(proxyAddresses, dialOpts...)
	if err != nil {
		logger.Error("ensurePeer failed to dial peer", tag.Error(err))
		return nil, err
	}
	logger.Debug("ensurePeer successfully dialed peer")

	m.streamsMu.Lock()
	ps := m.peers[peerNodeName]
	if ps == nil {
		logger.Debug("ensurePeer creating new peer state")
		ps = &peerState{conn: cc, receivers: make(map[peerStreamKey]*intraProxyStreamReceiver), senders: make(map[peerStreamKey]*intraProxyStreamSender), recvShutdown: make(map[peerStreamKey]channel.ShutdownOnce)}
		m.peers[peerNodeName] = ps
	} else {
		logger.Debug("ensurePeer updating existing peer state with new connection")
		old := ps.conn
		ps.conn = cc
		if old != nil {
			logger.Debug("ensurePeer closing old connection")
			_ = old.Close()
		}
		if ps.receivers == nil {
			ps.receivers = make(map[peerStreamKey]*intraProxyStreamReceiver)
		}
		if ps.senders == nil {
			ps.senders = make(map[peerStreamKey]*intraProxyStreamSender)
		}
		if ps.recvShutdown == nil {
			ps.recvShutdown = make(map[peerStreamKey]channel.ShutdownOnce)
		}
	}
	m.streamsMu.Unlock()
	return ps, nil
}

// ensureStream dials a peer proxy outbound server and opens a replication stream.
func (m *intraProxyManager) ensureStream(
	ctx context.Context,
	logger log.Logger,
	peerNodeName string,
	targetShard history.ClusterShardID,
	sourceShard history.ClusterShardID,
) error {
	logger.Debug("ensureStream")
	key := peerStreamKey{targetShard: targetShard, sourceShard: sourceShard}

	// Fast path: already exists
	m.streamsMu.RLock()
	if ps, ok := m.peers[peerNodeName]; ok && ps != nil {
		if r, ok2 := ps.receivers[key]; ok2 && r != nil && r.streamClient != nil {
			m.streamsMu.RUnlock()
			logger.Debug("ensureStream reused")
			return nil
		}
	}
	m.streamsMu.RUnlock()

	// Reuse shared connection per peer
	ps, err := m.ensurePeer(ctx, peerNodeName)
	if err != nil {
		logger.Error("Failed to ensure peer", tag.Error(err))
		return err
	}

	// Create receiver and register tracking
	recv := &intraProxyStreamReceiver{
		logger: log.With(m.logger,
			tag.NewStringTag("peerNodeName", peerNodeName),
			tag.NewStringTag("targetShardID", ClusterShardIDtoString(targetShard)),
			tag.NewStringTag("sourceShardID", ClusterShardIDtoString(sourceShard))),
		shardManager:  m.shardManager,
		intraMgr:      m,
		peerNodeName:  peerNodeName,
		targetShardID: targetShard,
		sourceShardID: sourceShard,
	}
	// initialize shutdown handle and register it for lifecycle management
	recv.shutdown = channel.NewShutdownOnce()
	m.streamsMu.Lock()
	ps.receivers[key] = recv
	ps.recvShutdown[key] = recv.shutdown
	m.streamsMu.Unlock()
	m.logger.Debug("intraProxyStreamReceiver added", tag.NewStringTag("peerNodeName", peerNodeName), tag.NewStringTag("key", fmt.Sprintf("%v", key)), tag.NewStringTag("receiver", recv.streamID))

	// Let the receiver open stream, register tracking, and start goroutines
	go func() {
		if err := recv.Run(ctx, m.shardManager, ps.conn); err != nil {
			m.logger.Error("intraProxyStreamReceiver.Run error", tag.Error(err))
		}
		// remove the receiver from the peer state
		m.streamsMu.Lock()
		delete(ps.receivers, key)
		delete(ps.recvShutdown, key)
		m.streamsMu.Unlock()
	}()
	return nil
}

// sendAck forwards an ACK to the specified peer stream (creates it on demand).
func (m *intraProxyManager) sendAck(
	ctx context.Context,
	peerNodeName string,
	clientShard history.ClusterShardID,
	serverShard history.ClusterShardID,
	req *adminservice.StreamWorkflowReplicationMessagesRequest,
) error {
	key := peerStreamKey{targetShard: clientShard, sourceShard: serverShard}
	m.streamsMu.RLock()
	defer m.streamsMu.RUnlock()
	if ps, ok := m.peers[peerNodeName]; ok && ps != nil {
		if r, ok2 := ps.receivers[key]; ok2 && r != nil && r.streamClient != nil {
			if err := r.sendAck(req); err != nil {
				m.logger.Error("Failed to send intra-proxy ACK", tag.Error(err))
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("peer not found")
}

// sendReplicationMessages sends replication messages to the peer via the server stream.
func (m *intraProxyManager) sendReplicationMessages(
	ctx context.Context,
	peerNodeName string,
	targetShard history.ClusterShardID,
	sourceShard history.ClusterShardID,
	resp *adminservice.StreamWorkflowReplicationMessagesResponse,
) error {
	key := peerStreamKey{targetShard: targetShard, sourceShard: sourceShard}
	logger := log.With(m.logger, tag.NewStringTag("task-target-shard", ClusterShardIDtoString(targetShard)), tag.NewStringTag("task-source-shard", ClusterShardIDtoString(sourceShard)))
	logger.Debug("sendReplicationMessages")
	defer logger.Debug("sendReplicationMessages finished")

	// Try server stream first with short retry/backoff to await registration
	deadline := time.Now().Add(2 * time.Second)
	backoff := 10 * time.Millisecond
	for {
		var sender *intraProxyStreamSender
		m.streamsMu.RLock()
		ps, ok := m.peers[peerNodeName]
		if ok && ps != nil && ps.senders != nil {
			logger.Debug("sendReplicationMessages senders for node", tag.NewStringTag("node", peerNodeName), tag.NewStringTag("senders", fmt.Sprintf("%v", ps.senders)))
			if s, ok2 := ps.senders[key]; ok2 && s != nil {
				sender = s
			}
		}
		m.streamsMu.RUnlock()
		logger.Debug("sendReplicationMessages sender", tag.NewStringTag("sender", fmt.Sprintf("%v", sender)))

		if sender != nil {
			if err := sender.sendReplicationMessages(resp); err != nil {
				logger.Error("Failed to send intra-proxy replication messages via server stream", tag.Error(err))
				return err
			}
			return nil
		}

		if time.Now().After(deadline) {
			break
		}
		time.Sleep(backoff)
		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}

	return fmt.Errorf("failed to send replication messages")
}

// closePeerLocked shuts down and removes all resources for a peer. Caller must hold m.streamsMu.
func (m *intraProxyManager) closePeerLocked(peer string, ps *peerState) {
	// Shutdown receivers and unregister client-side tracker entries
	for key, shut := range ps.recvShutdown {
		if shut != nil {
			shut.Shutdown()
		}
		st := GetGlobalStreamTracker()
		cliID := BuildIntraProxyReceiverStreamID(peer, key.targetShard, key.sourceShard)
		st.UnregisterStream(cliID)
		delete(ps.recvShutdown, key)
	}
	// Close client streams (receiver cleanup is handled by its own goroutine)
	for key := range ps.receivers {
		m.logger.Info("intraProxyStreamReceiver deleted", tag.NewStringTag("peerNodeName", peer), tag.NewStringTag("key", fmt.Sprintf("%v", key)), tag.NewStringTag("receiver", ps.receivers[key].streamID))
		delete(ps.receivers, key)
	}
	// Unregister server-side tracker entries
	for key := range ps.senders {
		st := GetGlobalStreamTracker()
		srvID := BuildIntraProxySenderStreamID(peer, key.targetShard, key.sourceShard)
		st.UnregisterStream(srvID)
		delete(ps.senders, key)
	}
	if ps.conn != nil {
		_ = ps.conn.Close()
		ps.conn = nil
	}
	delete(m.peers, peer)
}

// closePeerShardLocked shuts down and removes resources for a specific peer/shard pair. Caller must hold m.streamsMu.
func (m *intraProxyManager) closePeerShardLocked(peer string, ps *peerState, key peerStreamKey) {
	m.logger.Info("closePeerShardLocked", tag.NewStringTag("peer", peer), tag.NewStringTag("clientShard", ClusterShardIDtoString(key.targetShard)), tag.NewStringTag("serverShard", ClusterShardIDtoString(key.sourceShard)))
	if shut, ok := ps.recvShutdown[key]; ok && shut != nil {
		shut.Shutdown()
		st := GetGlobalStreamTracker()
		cliID := BuildIntraProxyReceiverStreamID(peer, key.targetShard, key.sourceShard)
		st.UnregisterStream(cliID)
		delete(ps.recvShutdown, key)
	}
	if r, ok := ps.receivers[key]; ok {
		// cancel stream context and attempt to close client send side
		if r.cancel != nil {
			r.cancel()
		}
		if r.streamClient != nil {
			_ = r.streamClient.CloseSend()
		}
		m.logger.Info("intraProxyStreamReceiver deleted", tag.NewStringTag("peerNodeName", peer), tag.NewStringTag("key", fmt.Sprintf("%v", key)), tag.NewStringTag("receiver", r.streamID))
		delete(ps.receivers, key)
	}
	st := GetGlobalStreamTracker()
	srvID := BuildIntraProxySenderStreamID(peer, key.targetShard, key.sourceShard)
	st.UnregisterStream(srvID)
	delete(ps.senders, key)
}

// ClosePeer closes and removes all resources for a specific peer.
func (m *intraProxyManager) ClosePeer(peer string) {
	m.streamsMu.Lock()
	defer m.streamsMu.Unlock()
	if ps, ok := m.peers[peer]; ok {
		m.closePeerLocked(peer, ps)
	}
}

// ClosePeerShard closes resources for a specific peer/shard pair.
func (m *intraProxyManager) ClosePeerShard(peer string, clientShard, serverShard history.ClusterShardID) {
	key := peerStreamKey{targetShard: clientShard, sourceShard: serverShard}
	m.streamsMu.Lock()
	defer m.streamsMu.Unlock()
	if ps, ok := m.peers[peer]; ok {
		m.closePeerShardLocked(peer, ps, key)
	}
}

func (m *intraProxyManager) Start() {
	m.logger.Info("intraProxyManager starting")
	defer m.logger.Info("intraProxyManager started")
	go func() {
		for {
			// timer
			timer := time.NewTimer(1 * time.Second)
			select {
			case <-timer.C:
				m.ReconcilePeerStreams("")
			case <-m.notifyCh:
				m.ReconcilePeerStreams("")
			}
		}
	}()
}

func (m *intraProxyManager) Notify() {
	select {
	case m.notifyCh <- struct{}{}:
	default:
	}
}

// ReconcilePeerStreams ensures receivers exist for desired (local shard, remote shard) pairs
// for a given peer and closes any sender/receiver not in the desired set.
// This mirrors the Temporal StreamReceiverMonitor approach.
func (m *intraProxyManager) ReconcilePeerStreams(peerNodeName string) {
	m.logger.Debug("ReconcilePeerStreams started", tag.NewStringTag("peerNodeName", peerNodeName))
	defer m.logger.Debug("ReconcilePeerStreams done", tag.NewStringTag("peerNodeName", peerNodeName))

	localShards := m.shardManager.GetLocalShards()
	remoteShards, err := m.shardManager.GetRemoteShardsForPeer(peerNodeName)
	if err != nil {
		m.logger.Error("Failed to get remote shards for peer", tag.Error(err))
		return
	}
	m.logger.Debug("ReconcilePeerStreams remote and local shards",
		tag.NewStringTag("peerNodeName", peerNodeName),
		tag.NewStringTag("remoteShards", fmt.Sprintf("%v", remoteShards)),
		tag.NewStringTag("localShards", fmt.Sprintf("%v", localShards)),
	)

	// Build desiredReceivers receiver set of cross-cluster pairs
	desiredReceivers := make(map[peerStreamKey]string)
	for _, l := range localShards {
		for peer, shards := range remoteShards {
			for _, r := range shards.Shards {
				if l.ClusterID == r.ID.ClusterID {
					continue
				}
				desiredReceivers[peerStreamKey{targetShard: l, sourceShard: r.ID}] = peer
			}
		}
	}

	// Build desiredSenders set: inverted direction of desiredReceivers
	// Senders exist when remote shard is the target and local shard is the source
	desiredSenders := make(map[peerStreamKey]string)
	for _, l := range localShards {
		for peer, shards := range remoteShards {
			for _, r := range shards.Shards {
				if l.ClusterID == r.ID.ClusterID {
					continue
				}
				desiredSenders[peerStreamKey{targetShard: r.ID, sourceShard: l}] = peer
			}
		}
	}

	m.logger.Debug("ReconcilePeerStreams desired receivers and senders", tag.NewStringTag("desiredReceivers", fmt.Sprintf("%v", desiredReceivers)), tag.NewStringTag("desiredSenders", fmt.Sprintf("%v", desiredSenders)))

	// Ensure all desired receivers exist
	for key := range desiredReceivers {
		m.EnsureReceiverForPeerShard(desiredReceivers[key], key.targetShard, key.sourceShard)
	}

	// Prune anything not desired
	check := func(peer string, ps *peerState) {
		// Collect keys to close for receivers
		var receiversToClose []peerStreamKey
		for key := range ps.receivers {
			if _, ok2 := desiredReceivers[key]; !ok2 {
				receiversToClose = append(receiversToClose, key)
			}
		}
		for _, key := range receiversToClose {
			m.closePeerShardLocked(peer, ps, key)
		}
		// Collect keys to close for senders
		var sendersToClose []peerStreamKey
		for key := range ps.senders {
			if _, ok2 := desiredSenders[key]; !ok2 {
				sendersToClose = append(sendersToClose, key)
			}
		}
		for _, key := range sendersToClose {
			m.closePeerShardLocked(peer, ps, key)
		}
	}

	m.streamsMu.Lock()
	if peerNodeName != "" {
		if ps, ok := m.peers[peerNodeName]; ok && ps != nil {
			check(peerNodeName, ps)
		}
	} else {
		for peer, ps := range m.peers {
			check(peer, ps)
		}
	}
	m.streamsMu.Unlock()
}
