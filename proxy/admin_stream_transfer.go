package proxy

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/metadata"

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
	logger = log.With(logger,
		tag.NewStringTag("source", ClusterShardIDtoString(sourceClusterShardID)),
		tag.NewStringTag("target", ClusterShardIDtoString(targetClusterShardID)))

	return &StreamForwarder{
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
	// simply forwarding target metadata
	outgoingContext := metadata.NewOutgoingContext(f.targetStreamServer.Context(), f.targetMetadata)
	outgoingContext, cancel := context.WithCancel(outgoingContext)
	defer cancel()

	// The underlying adminClient will try to grab a connection when we call StreamWorkflowReplicationMessages.
	// The connection is separately managed, so we want to see how long it takes to establish that conn.
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
	// this function.
	// Scenario 1: Remote disconnects. sourceStreamClient.Recv will return EOF. This unblocks forwardReplicationMessages and sets shutdownChan.
	//             forwardAck needs to be unblocked from targetStreamServer.Recv
	// Scenario 2: Local disconnects. targetStreamServer.Recv will return EOF. This unblocks forwardAck and sets shutdownChan.
	//             forwardReplicationMessages needs to be unblocked from sourceStreamClient.Recv
	f.shutdownChan = channel.NewShutdownOnce()
	var wg sync.WaitGroup
	wg.Add(2)
	go f.forwardAck(&wg)
	go f.forwardReplicationMessages(&wg)
	wg.Wait()

	metrics.AdminServiceStreamDuration.WithLabelValues(f.metricLabelValues...).Observe(time.Since(streamStartTime).Seconds())
	return nil
}

func (f *StreamForwarder) forwardReplicationMessages(wg *sync.WaitGroup) {
	defer func() {
		f.logger.Debug("Shutdown sourceStreamClient.Recv loop.")
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

func (f *StreamForwarder) forwardAck(wg *sync.WaitGroup) {
	defer func() {
		f.logger.Debug("Shutdown targetStreamServer.Recv loop.")
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
			f.logger.Debug("forwarding SyncReplicationState", tag.NewInt64("inclusive", attr.SyncReplicationState.GetInclusiveLowWatermark()))
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
