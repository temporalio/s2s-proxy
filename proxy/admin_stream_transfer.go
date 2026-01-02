package proxy

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	servercommon "go.temporal.io/server/common"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
)

type StreamRequestOrResponse interface {
	adminservice.StreamWorkflowReplicationMessagesRequest | adminservice.StreamWorkflowReplicationMessagesResponse
}
type ValueWithError[T StreamRequestOrResponse] struct {
	val *T
	err error
}
type recvable[T StreamRequestOrResponse] interface {
	Recv() (*T, error)
}

// startListener creates a channel of Recv() from the provided source. It is the job of the caller to cancel the context
// that will stop Recv(), or the goroutine created by this will block forever
func startListener[T StreamRequestOrResponse](
	receiver recvable[T],
	shutdownChan channel.ShutdownOnce,
) chan ValueWithError[T] {
	targetStreamServerData := make(chan ValueWithError[T])
	go func() {
		defer close(targetStreamServerData)
		for !shutdownChan.IsShutdown() {
			req, err := receiver.Recv()
			select {
			case targetStreamServerData <- ValueWithError[T]{val: req, err: err}:
			case <-shutdownChan.Channel():
				return
			}
		}
	}()
	return targetStreamServerData
}

type StreamForwarder struct {
	adminClient          adminservice.AdminServiceClient
	targetStreamServer   adminservice.AdminService_StreamWorkflowReplicationMessagesServer
	targetMetadata       metadata.MD
	sourceClusterShardID history.ClusterShardID
	targetClusterShardID history.ClusterShardID
	metricLabelValues    []string
	logger               log.Logger
	streamID             string

	sourceStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient
	shutdownChan       channel.ShutdownOnce
}

func newStreamForwarder(
	adminClient adminservice.AdminServiceClient,
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	targetMetadata metadata.MD,
	sourceClusterShardID history.ClusterShardID,
	targetClusterShardID history.ClusterShardID,
	metricLabelValues []string,
	logger log.Logger,
) *StreamForwarder {
	streamID := BuildForwarderStreamID(sourceClusterShardID, targetClusterShardID)
	logger = log.With(logger, tag.NewStringTag("streamID", streamID))
	return &StreamForwarder{
		streamID:             streamID,
		adminClient:          adminClient,
		targetStreamServer:   targetStreamServer,
		targetMetadata:       targetMetadata,
		sourceClusterShardID: sourceClusterShardID,
		targetClusterShardID: targetClusterShardID,
		metricLabelValues:    metricLabelValues,
		logger:               logger,
	}
}

// Run forwards messages between targetStreamServer and sourceStreamClient.
// It sets up bidirectional forwarding with proper shutdown handling.
// Returns the stream duration.
func (f *StreamForwarder) Run() error {
	f.logger = log.With(f.logger,
		tag.NewStringTag("role", "forwarder"),
		tag.NewStringTag("streamID", f.streamID),
	)

	// simply forwarding target metadata
	outgoingContext := metadata.NewOutgoingContext(f.targetStreamServer.Context(), f.targetMetadata)
	outgoingContext, cancel := context.WithCancel(outgoingContext)
	defer cancel()

	// The underlying adminClient will try to grab a connection when we call StreamWorkflowReplicationMessages.
	sourceStreamClient, err := f.adminClient.StreamWorkflowReplicationMessages(outgoingContext)
	if err != nil {
		f.logger.Error("remoteAdminServiceClient.StreamWorkflowReplicationMessages encountered error", tag.Error(err))
		return err
	}
	f.sourceStreamClient = sourceStreamClient

	// We succesfully got a stream connection, so mark the stream as active
	metrics.AdminServiceStreamsOpenedCount.WithLabelValues(f.metricLabelValues...).Inc()
	defer metrics.AdminServiceStreamsClosedCount.WithLabelValues(f.metricLabelValues...).Inc()
	streamStartTime := time.Now()

	// Register the forwarder stream here
	streamTracker := GetGlobalStreamTracker()
	sourceShard := ClusterShardIDtoString(f.sourceClusterShardID)
	targetShard := ClusterShardIDtoString(f.targetClusterShardID)
	streamTracker.RegisterStream(f.streamID, "StreamWorkflowReplicationMessages", "forwarder", sourceShard, targetShard, StreamRoleForwarder)
	defer streamTracker.UnregisterStream(f.streamID)

	// When one side of the stream dies, we want to tell the other side to hang up
	// (see https://stackoverflow.com/questions/68218469/how-to-un-wedge-go-grpc-bidi-streaming-server-from-the-blocking-recv-call)
	// One call to StreamWorkflowReplicationMessages establishes a one-way channel through the proxy from one server to another.
	// For an outbound stream, we have:
	// StreamWorkflowReplicationMessagesRequest
	// Local.Recv ===> Proxy ===> Remote.Send
	//  ^targetStreamServer    ^sourceStreamClient
	//
	// StreamWorkflowReplicationMessagesResponse
	// Local.Send <=== Proxy <=== Remote.Recv
	//  ^targetStreamServer   ^sourceStreamClient
	//
	// We can freely close sourceStreamClient with closeSend. gRPC will only cancel targetStreamServer when we return from
	// the service handler.
	// Scenario 1: Remote disconnects. sourceStreamClient.Recv will return EOF. This unblocks forwardReplicationMessages and sets shutdownChan.
	//             forwardAck needs to be unblocked from targetStreamServer.Recv
	// Scenario 2: Local disconnects. targetStreamServer.Recv will return EOF. This unblocks forwardAck and sets shutdownChan.
	//             forwardReplicationMessages needs to be unblocked from sourceStreamClient.Recv
	f.shutdownChan = channel.NewShutdownOnce()
	var wg sync.WaitGroup
	wg.Add(2)
	go f.forwardAcks(&wg)
	go f.forwardReplicationMessages(&wg)
	wg.Wait()

	metrics.AdminServiceStreamDuration.WithLabelValues(f.metricLabelValues...).Observe(time.Since(streamStartTime).Seconds())
	return nil
}

func (f *StreamForwarder) forwardReplicationMessages(wg *sync.WaitGroup) {
	f.logger.Info("proxyStreamForwarder forwardReplicationMessages started")
	defer f.logger.Info("proxyStreamForwarder forwardReplicationMessages finished")

	defer func() {
		f.shutdownChan.Shutdown()
		wg.Done()
	}()

	dataChan := startListener(f.sourceStreamClient, f.shutdownChan)

	for {
		var resp *adminservice.StreamWorkflowReplicationMessagesResponse
		var err error
		select {
		case <-f.shutdownChan.Channel():
			return
		case dataWithError := <-dataChan:
			resp = dataWithError.val
			err = dataWithError.err
		}
		if err == io.EOF {
			f.logger.Debug("sourceStreamClient.Recv encountered EOF", tag.Error(err))
			metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(f.metricLabelValues, "source")...).Inc()
			return
		}

		if err != nil {
			f.logger.Error("sourceStreamClient.Recv encountered error", tag.Error(err))
			return
		}
		switch attr := resp.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			f.logger.Debug("forwarding ReplicationMessages", tag.NewInt64("exclusive", attr.Messages.GetExclusiveHighWatermark()))

			msg := make([]string, 0, len(attr.Messages.ReplicationTasks))
			for i, task := range attr.Messages.ReplicationTasks {
				msg = append(msg, fmt.Sprintf("[%d]: %v", i, task.SourceTaskId))
			}
			f.logger.Debug(fmt.Sprintf("forwarding ReplicationMessages: exclusive %v, tasks: %v", attr.Messages.ExclusiveHighWatermark, strings.Join(msg, ", ")))

			streamTracker := GetGlobalStreamTracker()
			streamTracker.UpdateStreamReplicationMessages(f.streamID, attr.Messages.ExclusiveHighWatermark)

			if err = f.targetStreamServer.Send(resp); err != nil {
				if err != io.EOF {
					f.logger.Error("targetStreamServer.Send encountered error", tag.Error(err))
				} else {
					f.logger.Debug("targetStreamServer.Send encountered EOF", tag.Error(err))
					metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(f.metricLabelValues, "target")...).Inc()
				}
				return
			}
			metrics.AdminServiceStreamReqCount.WithLabelValues(f.metricLabelValues...).Inc()
		default:
			f.logger.Error("sourceStreamClient.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
				"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
			))))
			return
		}
	}
}

func (f *StreamForwarder) forwardAcks(wg *sync.WaitGroup) {
	defer func() {
		f.logger.Info("StreamForwarder forwardAck started")
		defer f.logger.Info("proxyStreamForwarder forwardAck finished")
		f.shutdownChan.Shutdown()
		var err error
		closeSent := make(chan struct{})
		go func() {
			err = f.sourceStreamClient.CloseSend()
			closeSent <- struct{}{}
		}()
		timeout := time.After(time.Second)
		select {
		case <-closeSent:
			break
		case <-timeout:
			err = fmt.Errorf("timed out waiting for source stream to close")
		}

		if err != nil {
			f.logger.Error("Failed to close sourceStreamClient", tag.Error(err))
		}
		wg.Done()
	}()

	dataChan := startListener(f.targetStreamServer, f.shutdownChan)

	for {
		var req *adminservice.StreamWorkflowReplicationMessagesRequest
		var err error
		select {
		case <-f.shutdownChan.Channel():
			return
		case valueWithError := <-dataChan:
			req = valueWithError.val
			err = valueWithError.err
		}
		if err == io.EOF {
			f.logger.Debug("targetStreamServer.Recv encountered EOF", tag.Error(err))
			metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(f.metricLabelValues, "target")...).Inc()
			return
		}

		if err != nil {
			f.logger.Error("targetStreamServer.Recv encountered error", tag.Error(err))
			return
		}

		switch attr := req.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState:
			f.logger.Debug(fmt.Sprintf("forwarding SyncReplicationState: inclusive %v, attr: %v", attr.SyncReplicationState.InclusiveLowWatermark, attr))

			var watermarkTime *time.Time
			if attr.SyncReplicationState.InclusiveLowWatermarkTime != nil {
				t := attr.SyncReplicationState.InclusiveLowWatermarkTime.AsTime()
				watermarkTime = &t
			}
			streamTracker := GetGlobalStreamTracker()
			streamTracker.UpdateStreamSyncReplicationState(f.streamID, attr.SyncReplicationState.InclusiveLowWatermark, watermarkTime)
			if err = f.sourceStreamClient.Send(req); err != nil {
				if err != io.EOF {
					f.logger.Error("sourceStreamClient.Send encountered error", tag.Error(err))
				} else {
					f.logger.Debug("sourceStreamClient.Send encountered EOF", tag.Error(err))
					metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(f.metricLabelValues, "source")...).Inc()
				}
				return
			}
			metrics.AdminServiceStreamRespCount.WithLabelValues(f.metricLabelValues...).Inc()
		default:
			f.logger.Error("targetStreamServer.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
				"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
			))))
			return
		}
	}
}

// handleStream handles the routing logic for StreamWorkflowReplicationMessages based on shard count mode.
func handleStream(
	streamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	targetMetadata metadata.MD,
	sourceClusterShardID history.ClusterShardID,
	targetClusterShardID history.ClusterShardID,
	logger log.Logger,
	shardCountConfig config.ShardCountConfig,
	lcmParameters LCMParameters,
	routingParameters RoutingParameters,
	adminClient adminservice.AdminServiceClient,
	adminClientReverse adminservice.AdminServiceClient,
	shardManager ShardManager,
	metricLabelValues []string,
) error {
	switch shardCountConfig.Mode {
	case config.ShardCountLCM:
		// Arbitrary shard count support.
		//
		// Temporal only supports shard counts where one shard count is an even multiple of the other.
		// The trick in this mode is the proxy will present the Least Common Multiple of both cluster shard counts.
		// Temporal establishes outbound replication streams to the proxy for all unique shard id pairs between
		// itself and the proxy's shard count. Then the proxy directly forwards those streams along to the target
		// cluster, remapping proxy stream shard ids to the target cluster shard ids.
		newTargetShardID := history.ClusterShardID{
			ClusterID: targetClusterShardID.ClusterID,
			ShardID:   sourceClusterShardID.ShardID, // proxy fake shard id
		}
		newSourceShardID := history.ClusterShardID{
			ClusterID: sourceClusterShardID.ClusterID,
		}
		// Remap shard id using the pre-calculated target shard count.
		newSourceShardID.ShardID = mapShardIDUnique(lcmParameters.LCM, lcmParameters.TargetShardCount, sourceClusterShardID.ShardID)

		logger = log.With(logger,
			tag.NewStringTag("newTarget", ClusterShardIDtoString(newTargetShardID)),
			tag.NewStringTag("newSource", ClusterShardIDtoString(newSourceShardID)))

		// Maybe there's a cleaner way. Trying to preserve any other metadata.
		targetMetadata.Set(history.MetadataKeyClientClusterID, strconv.Itoa(int(newTargetShardID.ClusterID)))
		targetMetadata.Set(history.MetadataKeyClientShardID, strconv.Itoa(int(newTargetShardID.ShardID)))
		targetMetadata.Set(history.MetadataKeyServerClusterID, strconv.Itoa(int(newSourceShardID.ClusterID)))
		targetMetadata.Set(history.MetadataKeyServerShardID, strconv.Itoa(int(newSourceShardID.ShardID)))
	case config.ShardCountRouting:
		isIntraProxy := common.IsIntraProxy(streamServer.Context())
		if isIntraProxy {
			return streamIntraProxyRouting(logger, streamServer, sourceClusterShardID, targetClusterShardID, shardManager)
		}
		return streamRouting(logger, streamServer, sourceClusterShardID, targetClusterShardID, shardManager, adminClientReverse, routingParameters)
	}

	forwarder := newStreamForwarder(
		adminClient,
		streamServer,
		targetMetadata,
		sourceClusterShardID,
		targetClusterShardID,
		metricLabelValues,
		logger,
	)
	return forwarder.Run()
}

func streamIntraProxyRouting(
	logger log.Logger,
	streamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	sourceShardID history.ClusterShardID,
	targetShardID history.ClusterShardID,
	shardManager ShardManager,
) error {
	logger.Info("streamIntraProxyRouting started")
	defer logger.Info("streamIntraProxyRouting finished")

	// Determine remote peer identity from intra-proxy headers
	peerNodeName := ""
	if md, ok := metadata.FromIncomingContext(streamServer.Context()); ok {
		vals := md.Get(common.IntraProxyOriginProxyIDHeader)
		if len(vals) > 0 {
			peerNodeName = vals[0]
		}
	}

	// Only allow intra-proxy when at least one shard is local to this proxy instance
	isLocalSource := shardManager.IsLocalShard(sourceShardID)
	isLocalTarget := shardManager.IsLocalShard(targetShardID)
	if isLocalTarget || !isLocalSource {
		logger.Info("Skipping intra-proxy between two local shards or two remote shards. Client may use outdated shard info.",
			tag.NewBoolTag("isLocalSource", isLocalSource),
			tag.NewBoolTag("isLocalTarget", isLocalTarget),
		)
		return nil
	}

	// Sender: handle ACKs coming from peer and forward to original owner
	sender := &intraProxyStreamSender{
		logger:        logger,
		shardManager:  shardManager,
		peerNodeName:  peerNodeName,
		sourceShardID: sourceShardID,
		targetShardID: targetShardID,
	}

	shutdownChan := channel.NewShutdownOnce()
	go func() {
		if err := sender.Run(streamServer, shutdownChan); err != nil {
			logger.Error("intraProxyStreamSender.Run error", tag.Error(err))
		}
	}()
	<-shutdownChan.Channel()
	return nil
}

func streamRouting(
	logger log.Logger,
	streamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	sourceShardID history.ClusterShardID,
	targetShardID history.ClusterShardID,
	shardManager ShardManager,
	adminClientReverse adminservice.AdminServiceClient,
	routingParameters RoutingParameters,
) error {
	logger.Info("streamRouting started")
	defer logger.Info("streamRouting stopped")

	// client: stream receiver
	// server: stream sender
	proxyStreamSender := &proxyStreamSender{
		logger:         logger,
		shardManager:   shardManager,
		sourceShardID:  sourceShardID,
		targetShardID:  targetShardID,
		directionLabel: routingParameters.DirectionLabel,
	}

	proxyStreamReceiver := &proxyStreamReceiver{
		logger:          logger,
		shardManager:    shardManager,
		adminClient:     adminClientReverse,
		localShardCount: routingParameters.RoutingLocalShardCount,
		sourceShardID:   targetShardID, // reverse direction
		targetShardID:   sourceShardID, // reverse direction
		directionLabel:  routingParameters.DirectionLabel,
	}

	shutdownChan := channel.NewShutdownOnce()
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		proxyStreamSender.Run(streamServer, shutdownChan)
	}()
	go func() {
		defer wg.Done()
		proxyStreamReceiver.Run(shutdownChan)
	}()
	wg.Wait()

	return nil
}

func mapShardIDUnique(sourceShardCount, targetShardCount, sourceShardID int32) int32 {
	targetShardID := servercommon.MapShardID(sourceShardCount, targetShardCount, sourceShardID)
	if len(targetShardID) != 1 {
		panic(fmt.Sprintf("remapping shard count error: sourceShardCount=%d targetShardCount=%d sourceShardID=%d targetShardID=%v\n",
			sourceShardCount, targetShardCount, sourceShardID, targetShardID))
	}
	return targetShardID[0]
}
