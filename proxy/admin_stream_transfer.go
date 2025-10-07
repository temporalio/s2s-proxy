package proxy

import (
	"fmt"
	"io"
	"sync"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

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

func transferSourceToTarget(
	sourceStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	wg *sync.WaitGroup,
	shutdownChan channel.ShutdownOnce,
	metricLabelValues []string,
	logger log.Logger,
) {
	defer func() {
		logger.Debug("Shutdown sourceStreamClient.Recv loop.")
		shutdownChan.Shutdown()
		wg.Done()
	}()

	dataChan := startListener(sourceStreamClient, shutdownChan)

	for {
		var resp *adminservice.StreamWorkflowReplicationMessagesResponse
		var err error
		select {
		case <-shutdownChan.Channel():
			return
		case dataWithError := <-dataChan:
			resp = dataWithError.val
			err = dataWithError.err
		}
		if err == io.EOF {
			logger.Debug("sourceStreamClient.Recv encountered EOF", tag.Error(err))
			metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(metricLabelValues, "source")...).Inc()
			return
		}

		if err != nil {
			logger.Error("sourceStreamClient.Recv encountered error", tag.Error(err))
			return
		}
		switch attr := resp.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			logger.Debug("forwarding ReplicationMessages", tag.NewInt64("exclusive", attr.Messages.GetExclusiveHighWatermark()))

			if err = targetStreamServer.Send(resp); err != nil {
				if err != io.EOF {
					logger.Error("targetStreamServer.Send encountered error", tag.Error(err))
				} else {
					logger.Debug("targetStreamServer.Send encountered EOF", tag.Error(err))
					metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(metricLabelValues, "target")...).Inc()
				}
				return
			}
			metrics.AdminServiceStreamReqCount.WithLabelValues(metricLabelValues...).Inc()
		default:
			logger.Error("sourceStreamClient.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
				"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
			))))
			return
		}
	}
}

func transferTargetToSource(
	sourceStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	wg *sync.WaitGroup,
	shutdownChan channel.ShutdownOnce,
	metricLabelValues []string,
	logger log.Logger,
) {
	defer func() {
		logger.Debug("Shutdown targetStreamServer.Recv loop.")
		shutdownChan.Shutdown()
		var err error
		closeSent := make(chan struct{})
		go func() {
			err = sourceStreamClient.CloseSend()
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
			logger.Error("Failed to close sourceStreamClient", tag.Error(err))
		}
		wg.Done()
	}()

	dataChan := startListener(targetStreamServer, shutdownChan)

	for {
		var req *adminservice.StreamWorkflowReplicationMessagesRequest
		var err error
		select {
		case <-shutdownChan.Channel():
			return
		case valueWithError := <-dataChan:
			req = valueWithError.val
			err = valueWithError.err
		}
		if err == io.EOF {
			logger.Debug("targetStreamServer.Recv encountered EOF", tag.Error(err))
			metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(metricLabelValues, "target")...).Inc()
			return
		}

		if err != nil {
			logger.Error("targetStreamServer.Recv encountered error", tag.Error(err))
			return
		}

		switch attr := req.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState:
			logger.Debug("forwarding SyncReplicationState", tag.NewInt64("inclusive", attr.SyncReplicationState.GetInclusiveLowWatermark()))
			if err = sourceStreamClient.Send(req); err != nil {
				if err != io.EOF {
					logger.Error("sourceStreamClient.Send encountered error", tag.Error(err))
				} else {
					logger.Debug("sourceStreamClient.Send encountered EOF", tag.Error(err))
					metrics.AdminServiceStreamTerminatedCount.WithLabelValues(append(metricLabelValues, "source")...).Inc()
				}
				return
			}
			metrics.AdminServiceStreamRespCount.WithLabelValues(metricLabelValues...).Inc()
		default:
			logger.Error("targetStreamServer.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
				"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
			))))
			return
		}
	}
}
