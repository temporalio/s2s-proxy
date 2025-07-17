package proxy

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/client"
	adminclient "github.com/temporalio/s2s-proxy/client/admin"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	adminServiceProxyServer struct {
		adminservice.UnimplementedAdminServiceServer
		adminClient adminservice.AdminServiceClient
		logger      log.Logger
		proxyOptions
	}
)

var openStreams atomic.Int32

const MAX_STREAMS = 1025

func NewAdminServiceProxyServer(
	serviceName string,
	clientConfig config.ProxyClientConfig,
	clientFactory client.ClientFactory,
	opts proxyOptions,
	logger log.Logger,
) adminservice.AdminServiceServer {
	logger = log.With(logger, common.ServiceTag(serviceName))
	clientProvider := client.NewClientProvider(clientConfig, clientFactory, logger)
	return &adminServiceProxyServer{
		adminClient:  adminclient.NewLazyClient(clientProvider),
		logger:       logger,
		proxyOptions: opts,
	}
}

// Functions in this section reimplement or wrap the underlying client.
// Functions in the next section are passthrough.

func (s *adminServiceProxyServer) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	if !common.IsRequestTranslationDisabled(ctx) {
		if outbound := s.Config.Outbound; s.IsInbound && outbound != nil && len(outbound.Server.ExternalAddress) > 0 {
			// Override this address so that cross-cluster connections flow through the proxy.
			// Use a separate "external address" config option because the outbound.listenerAddress may not be routable
			// from the local temporal server, or the proxy may be deployed behind a load balancer.
			in0.FrontendAddress = outbound.Server.ExternalAddress
		}
	}
	return s.adminClient.AddOrUpdateRemoteCluster(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	resp, err := s.adminClient.DescribeCluster(ctx, in0)
	if common.IsRequestTranslationDisabled(ctx) {
		return resp, err
	}

	var overrides *config.APIOverridesConfig
	if s.IsInbound {
		if s.Config.Inbound != nil {
			overrides = s.Config.Inbound.APIOverrides
		}
	} else {
		if s.Config.Outbound != nil {
			overrides = s.Config.Outbound.APIOverrides
		}
	}

	if overrides != nil && overrides.AdminSerivce.DescribeCluster != nil {
		responseOverride := overrides.AdminSerivce.DescribeCluster.Response
		if resp != nil && responseOverride.FailoverVersionIncrement != nil {
			resp.FailoverVersionIncrement = *responseOverride.FailoverVersionIncrement
		}
	}

	return resp, err
}

// ClusterShardIDtoString stringifies a shard ID for logging
func ClusterShardIDtoString(sd history.ClusterShardID) string {
	return fmt.Sprintf("(id: %d, shard: %d)", sd.ClusterID, sd.ShardID)
}

// The inbound connection establishes an HTTP/2 stream. gRPC passes us a stream that represents the initiating server,
// and we can freely Send and Recv on that "server". Because this is a proxy, we also establish a bidirectional
// stream using our configured adminClient. When we Recv on the initiator, we Send to the client.
// When we Recv on the client, we Send to the initator
func (s *adminServiceProxyServer) StreamWorkflowReplicationMessages(
	initiatingServerStream adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	// Record streams active
	directionLabel := "inbound"
	if !s.IsInbound {
		directionLabel = "outbound"
	}
	metrics.AdminServiceStreamsOpenedCount.WithLabelValues(directionLabel).Inc()
	defer metrics.AdminServiceStreamsClosedCount.WithLabelValues(directionLabel).Inc()
	streamsActiveGauge := metrics.AdminServiceStreamsActive.WithLabelValues(directionLabel)
	streamsActiveGauge.Inc()
	defer streamsActiveGauge.Dec()
	var checkStreams int32
	// Pessimistic spinlock here strictly enforces MAX_STREAMS even under high connect load
	for {
		checkStreams = openStreams.Add(1)
		metrics.AdminServiceStreamsMeterGauge.WithLabelValues(directionLabel).Set(float64(checkStreams))
		// Putting the defer here is cleaner than trying to carry the decision variables down to the cleanup function
		defer func() {
			newVal := openStreams.Add(-1)
			metrics.AdminServiceStreamsMeterGauge.WithLabelValues(directionLabel).Set(float64(newVal))
		}()
		break
	}
	if checkStreams >= MAX_STREAMS {
		metrics.AdminServiceStreamsRejectedCount.WithLabelValues(directionLabel).Inc()
		return status.Errorf(codes.ResourceExhausted, "too many streams registered. Please retry")
	} else {
		metrics.AdminServiceStreamsAllowedGauge.WithLabelValues(directionLabel).Set(float64(checkStreams))
	}
	defer log.CapturePanic(s.logger, &retError)

	targetMetadata, ok := metadata.FromIncomingContext(initiatingServerStream.Context())
	if !ok {
		return serviceerror.NewInvalidArgument("missing cluster & shard ID metadata")
	}
	targetClusterShardID, sourceClusterShardID, err := history.DecodeClusterShardMD(
		headers.NewGRPCHeaderGetter(initiatingServerStream.Context()),
	)
	if err != nil {
		return err
	}

	logger := log.With(s.logger,
		tag.NewStringTag("source", ClusterShardIDtoString(sourceClusterShardID)),
		tag.NewStringTag("target", ClusterShardIDtoString(targetClusterShardID)))

	// simply forwarding target metadata
	outgoingContext := metadata.NewOutgoingContext(initiatingServerStream.Context(), targetMetadata)
	outgoingContext, cancel := context.WithCancel(outgoingContext)
	defer cancel()

	destinationStreamClient, err := s.adminClient.StreamWorkflowReplicationMessages(outgoingContext)
	if err != nil {
		logger.Error("remoteAdminServiceClient.StreamWorkflowReplicationMessages encountered error", tag.Error(err))
		return err
	}

	// Close the connection after this many requests have been handled. Jitter to more evenly balance clients using round-robin
	messagesBeforeClose := 120 + rand.IntN(120)
	connectInitiatorToDestination(initiatingServerStream, destinationStreamClient, messagesBeforeClose, directionLabel, logger)

	// For streaming returns, returning nil out of this function will send io.EOF to the stream
	return nil
}

func connectInitiatorToDestination(initiatingServerStream adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	destinationStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	messagesBeforeClose int,
	directionLabel string,
	logger log.Logger,
) {
	// Put each stream direction in its own goroutine so that if one side breaks, we can close both sides without waiting
	shutdownChan := channel.NewShutdownOnce()
	var wg sync.WaitGroup
	wg.Add(2)
	// Initiator -> client (targetStreamServer) recv loop
	go transferInitiatorToDestination(shutdownChan, destinationStreamClient, initiatingServerStream, &wg, messagesBeforeClose, directionLabel, logger)
	// Upstream (sourceStreamClient) recv loop
	go transferDestinationToInitiator(shutdownChan, destinationStreamClient, initiatingServerStream, &wg, directionLabel, logger)
	wg.Wait()
}

type ReplicationMessageRequestOrError struct {
	req *adminservice.StreamWorkflowReplicationMessagesRequest
	err error
}
type ReplicationMessageResponseOrError struct {
	resp *adminservice.StreamWorkflowReplicationMessagesResponse
	err  error
}

// transferInitiatorToDestination listens to the initiating connection and retransmits Requests to the destination connection.
// Keep in mind there are two gRPC servers, "inbound" and "outbound". On inbound, the initiator is the remote Temporal.
// On outbound, the initiator is the local Temporal and the destination is the remote Temporal.
// TODO: These two directions are ALMOST identical. We could probably do something with generics here
func transferInitiatorToDestination(shutdownChan channel.ShutdownOnce,
	destinationStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	initiatingServerStream adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	wg *sync.WaitGroup,
	messagesBeforeClose int,
	directionLabel string,
	logger log.Logger,
) {
	defer func() {
		logger.Info("Shutdown targetStreamServer.Recv loop.")
		shutdownChan.Shutdown()
		var err error
		closeSent := make(chan struct{})
		go func() {
			err = destinationStreamClient.CloseSend()
			closeSent <- struct{}{}
		}()
		timeout := time.After(30 * time.Second)
		select {
		case <-closeSent:
			break
		case <-timeout:
			err = fmt.Errorf("timed out waiting for destination stream to close")
		}

		if err != nil {
			logger.Error("Failed to close sourceStreamClient", tag.Error(err))
		}
		wg.Done()
	}()

	messagesHandled := 0
	ch := make(chan ReplicationMessageRequestOrError)
	for !shutdownChan.IsShutdown() {
		go func() {
			req, err := initiatingServerStream.Recv()
			ch <- ReplicationMessageRequestOrError{req: req, err: err}
		}()
		var req *adminservice.StreamWorkflowReplicationMessagesRequest
		var err error
		select {
		case callValue := <-ch:
			metrics.AdminServiceStreamReqCount.WithLabelValues(directionLabel).Inc()
			req = callValue.req
			err = callValue.err
		case <-shutdownChan.Channel():
			return
		}
		if err == io.EOF {
			logger.Info("initiatingServerStream.Recv encountered EOF", tag.Error(err))
			return
		}

		if err != nil {
			status := status.Convert(err)
			if status.Code() == codes.Canceled {
				metrics.AdminServiceStreamsClientHangupCount.WithLabelValues(directionLabel).Inc()
			} else {
				logger.Error("destinationStreamClient.Recv encountered unknown error", tag.Error(err), tag.NewStringTag("grpc_status", status.String()))
			}
			return
		}

		switch attr := req.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState:
			logger.Debug(fmt.Sprintf("forwarding SyncReplicationState: inclusive %v", attr.SyncReplicationState.InclusiveLowWatermark))
			if err = destinationStreamClient.Send(req); err != nil {
				if err != io.EOF {
					logger.Error("sourceStreamClient.Send encountered error", tag.Error(err))
				} else {
					logger.Info("sourceStreamClient.Send encountered EOF", tag.Error(err))
				}
				return
			}
		default:
			logger.Error("targetStreamServer.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
				"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
			))))
			return
		}
		// Close the channels after messagesBeforeClose messages have been passed. This helps
		// avoid clients maxing out a single server
		messagesHandled++
		metrics.AdminServiceStreamsMessagesHandledGauge.WithLabelValues(directionLabel).Set(float64(messagesHandled))
		// if messagesHandled > messagesBeforeClose {
		// metrics.ForceDisconnectCount.WithLabelValues(directionLabel).Inc()
		// shutdownChan.Shutdown()
		// }
	}
}

// transferDestinationToInitiator listens to the destination connection and retransmits Responses to the initiating connection.
// Keep in mind there are two gRPC servers, "inbound" and "outbound". On inbound, the initiator is the remote Temporal.
// On outbound, the initiator is the local Temporal and the destination is the remote Temporal.
func transferDestinationToInitiator(shutdownChan channel.ShutdownOnce,
	destinationStreamClient adminservice.AdminService_StreamWorkflowReplicationMessagesClient,
	initiatingServerStream adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
	wg *sync.WaitGroup,
	directionLabel string,
	logger log.Logger,
) {
	defer func() {
		logger.Info("Shutdown destinationStreamClient.Recv loop.")
		shutdownChan.Shutdown()
		wg.Done()
	}()

	ch := make(chan ReplicationMessageResponseOrError)
	for !shutdownChan.IsShutdown() {
		go func() {
			resp, err := destinationStreamClient.Recv()
			ch <- ReplicationMessageResponseOrError{resp, err}
		}()
		var resp *adminservice.StreamWorkflowReplicationMessagesResponse
		var err error
		select {
		case callValue := <-ch:
			metrics.AdminServiceStreamRespCount.WithLabelValues(directionLabel).Inc()
			resp = callValue.resp
			err = callValue.err
		case <-shutdownChan.Channel():
			return
		}
		if err == io.EOF {
			logger.Info("destinationStreamClient.Recv encountered EOF", tag.Error(err))
			return
		}

		if err != nil {
			status := status.Convert(err)
			if status.Code() == codes.Canceled {
				metrics.AdminServiceStreamsClientHangupCount.WithLabelValues(directionLabel).Inc()
			} else {
				logger.Error("destinationStreamClient.Recv encountered unknown error", tag.Error(err), tag.NewStringTag("grpc_status", status.String()))
			}
			return
		}
		switch attr := resp.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			logger.Debug(fmt.Sprintf("forwarding ReplicationMessages: exclusive %v", attr.Messages.ExclusiveHighWatermark))
			if err = initiatingServerStream.Send(resp); err != nil {
				if err != io.EOF {
					logger.Error("targetStreamServer.Send encountered error", tag.Error(err))
				} else {
					logger.Info("targetStreamServer.Send encountered EOF", tag.Error(err))
				}
				return
			}
		default:
			logger.Error("destinationStreamClient.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
				"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
			))))
			return
		}
	}
}

// Passthrough APIs below this point

func (s *adminServiceProxyServer) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	return s.adminClient.AddSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	return s.adminClient.AddTasks(ctx, in0)
}

func (s *adminServiceProxyServer) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	return s.adminClient.CancelDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	return s.adminClient.CloseShard(ctx, in0)
}

func (s *adminServiceProxyServer) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	return s.adminClient.DeleteWorkflowExecution(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	return s.adminClient.DescribeDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	return s.adminClient.DescribeHistoryHost(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	return s.adminClient.DescribeMutableState(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	return s.adminClient.GetDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return s.adminClient.GetDLQReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	return s.adminClient.GetDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	return s.adminClient.GetNamespace(ctx, in0)
}

func (s *adminServiceProxyServer) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	return s.adminClient.GetNamespaceReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	return s.adminClient.GetReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	return s.adminClient.GetSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	return s.adminClient.GetShard(ctx, in0)
}

func (s *adminServiceProxyServer) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	return s.adminClient.GetTaskQueueTasks(ctx, in0)
}

func (s *adminServiceProxyServer) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return s.adminClient.GetWorkflowExecutionRawHistory(ctx, in0)
}

func (s *adminServiceProxyServer) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return s.adminClient.GetWorkflowExecutionRawHistoryV2(ctx, in0)
}

func (s *adminServiceProxyServer) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	return s.adminClient.ImportWorkflowExecution(ctx, in0)
}

func (s *adminServiceProxyServer) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	return s.adminClient.ListClusterMembers(ctx, in0)
}

func (s *adminServiceProxyServer) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	return s.adminClient.ListClusters(ctx, in0)
}

func (s *adminServiceProxyServer) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	return s.adminClient.ListHistoryTasks(ctx, in0)
}

func (s *adminServiceProxyServer) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	return s.adminClient.ListQueues(ctx, in0)
}

func (s *adminServiceProxyServer) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	return s.adminClient.MergeDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	return s.adminClient.MergeDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	return s.adminClient.PurgeDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	return s.adminClient.PurgeDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	return s.adminClient.ReapplyEvents(ctx, in0)
}

func (s *adminServiceProxyServer) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	return s.adminClient.RebuildMutableState(ctx, in0)
}

func (s *adminServiceProxyServer) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	return s.adminClient.RefreshWorkflowTasks(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	return s.adminClient.RemoveRemoteCluster(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	return s.adminClient.RemoveSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	return s.adminClient.RemoveTask(ctx, in0)
}

func (s *adminServiceProxyServer) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	return s.adminClient.ResendReplicationTasks(ctx, in0)
}
