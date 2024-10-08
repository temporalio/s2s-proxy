package proxy

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/client"
	adminclient "github.com/temporalio/s2s-proxy/client/admin"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
)

type (
	adminServiceProxyServer struct {
		adminservice.UnimplementedAdminServiceServer
		adminClient adminservice.AdminServiceClient
		logger      log.Logger
		access      *auth.AccessControl
		proxyOptions
	}
)

func NewAdminServiceProxyServer(
	serviceName string,
	clientConfig config.ClientConfig,
	clientFactory client.ClientFactory,
	opts proxyOptions,
	logger log.Logger,
) adminservice.AdminServiceServer {
	logger = log.With(logger, common.ServiceTag(serviceName))
	clientProvider := client.NewClientProvider(clientConfig, clientFactory, logger)

	var allowedList []string
	if opts.IsInbound {
		if inbound := opts.Config.Inbound; inbound != nil && inbound.ACLPolicy != nil {
			allowedList = inbound.ACLPolicy.Migration.AllowedMethods.AdminService
		}
	}

	return &adminServiceProxyServer{
		adminClient:  adminclient.NewLazyClient(clientProvider),
		logger:       logger,
		proxyOptions: opts,
		access:       auth.NewAccesControl(allowedList),
	}
}

func (s *adminServiceProxyServer) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	if !s.access.IsAllowed("AddOrUpdateRemoteCluster") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method AddSearchAttributes is not allowed.")
	}

	if outbound := s.Config.Outbound; s.IsInbound && outbound != nil && len(outbound.Server.ExternalAddress) > 0 {
		// Override this address so that cross-cluster connections flow through the proxy.
		// Use a separate "external address" config option because the outbound.listenerAddress may not be routable
		// from the local temporal server, or the proxy may be deployed behind a load balancer.
		in0.FrontendAddress = outbound.Server.ExternalAddress
	}
	return s.adminClient.AddOrUpdateRemoteCluster(ctx, in0)
}

func (s *adminServiceProxyServer) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	if !s.access.IsAllowed("AddSearchAttributes") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method AddSearchAttributes is not allowed.")
	}

	return s.adminClient.AddSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	if !s.access.IsAllowed("AddTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method AddTasks is not allowed.")
	}

	return s.adminClient.AddTasks(ctx, in0)
}

func (s *adminServiceProxyServer) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	if !s.access.IsAllowed("CancelDLQJob") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method CancelDLQJob is not allowed.")
	}

	return s.adminClient.CancelDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	if !s.access.IsAllowed("CloseShard") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method CloseShard is not allowed.")
	}

	return s.adminClient.CloseShard(ctx, in0)
}

func (s *adminServiceProxyServer) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	if !s.access.IsAllowed("DeleteWorkflowExecution") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
	}

	return s.adminClient.DeleteWorkflowExecution(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	if !s.access.IsAllowed("DescribeCluster") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeCluster is not allowed.")
	}

	return s.adminClient.DescribeCluster(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	if !s.access.IsAllowed("DescribeDLQJob") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeDLQJob is not allowed.")
	}

	return s.adminClient.DescribeDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	if !s.access.IsAllowed("DescribeHistoryHost") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeHistoryHost is not allowed.")
	}

	return s.adminClient.DescribeHistoryHost(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	if !s.access.IsAllowed("DescribeMutableState") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeMutableState is not allowed.")
	}

	return s.adminClient.DescribeMutableState(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	if !s.access.IsAllowed("GetDLQMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQMessages is not allowed.")
	}

	return s.adminClient.GetDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	if !s.access.IsAllowed("GetDLQReplicationMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQReplicationMessages is not allowed.")
	}

	return s.adminClient.GetDLQReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	if !s.access.IsAllowed("GetDLQTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQTasks is not allowed.")
	}

	return s.adminClient.GetDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	if !s.access.IsAllowed("GetNamespace") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespace is not allowed.")
	}

	return s.adminClient.GetNamespace(ctx, in0)
}

func (s *adminServiceProxyServer) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	if !s.access.IsAllowed("GetNamespaceReplicationMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespaceReplicationMessages is not allowed.")
	}

	return s.adminClient.GetNamespaceReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	if !s.access.IsAllowed("GetReplicationMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetReplicationMessages is not allowed.")
	}

	return s.adminClient.GetReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	if !s.access.IsAllowed("GetSearchAttributes") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSearchAttributes is not allowed.")
	}

	return s.adminClient.GetSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	if !s.access.IsAllowed("GetShard") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetShard is not allowed.")
	}

	return s.adminClient.GetShard(ctx, in0)
}

func (s *adminServiceProxyServer) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	if !s.access.IsAllowed("GetTaskQueueTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetTaskQueueTasks is not allowed.")
	}

	return s.adminClient.GetTaskQueueTasks(ctx, in0)
}

func (s *adminServiceProxyServer) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	if !s.access.IsAllowed("GetWorkflowExecutionRawHistory") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistory is not allowed.")
	}

	return s.adminClient.GetWorkflowExecutionRawHistory(ctx, in0)
}

func (s *adminServiceProxyServer) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	if !s.access.IsAllowed("GetWorkflowExecutionRawHistoryV2") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistoryV2 is not allowed.")
	}

	return s.adminClient.GetWorkflowExecutionRawHistoryV2(ctx, in0)
}

func (s *adminServiceProxyServer) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	if !s.access.IsAllowed("ImportWorkflowExecution") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ImportWorkflowExecution is not allowed.")
	}

	return s.adminClient.ImportWorkflowExecution(ctx, in0)
}

func (s *adminServiceProxyServer) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	if !s.access.IsAllowed("ListClusterMembers") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
	}

	return s.adminClient.ListClusterMembers(ctx, in0)
}

func (s *adminServiceProxyServer) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	if !s.access.IsAllowed("ListClusters") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusters is not allowed.")
	}

	return s.adminClient.ListClusters(ctx, in0)
}

func (s *adminServiceProxyServer) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	if !s.access.IsAllowed("ListHistoryTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListHistoryTasks is not allowed.")
	}

	return s.adminClient.ListHistoryTasks(ctx, in0)
}

func (s *adminServiceProxyServer) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	if !s.access.IsAllowed("ListQueues") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListQueues is not allowed.")
	}

	return s.adminClient.ListQueues(ctx, in0)
}

func (s *adminServiceProxyServer) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	if !s.access.IsAllowed("MergeDLQMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQMessages is not allowed.")
	}

	return s.adminClient.MergeDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	if !s.access.IsAllowed("MergeDLQTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQTasks is not allowed.")
	}

	return s.adminClient.MergeDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	if !s.access.IsAllowed("PurgeDLQMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQMessages is not allowed.")
	}

	return s.adminClient.PurgeDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	if !s.access.IsAllowed("PurgeDLQTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQTasks is not allowed.")
	}

	return s.adminClient.PurgeDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	if !s.access.IsAllowed("ReapplyEvents") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ReapplyEvents is not allowed.")
	}

	return s.adminClient.ReapplyEvents(ctx, in0)
}

func (s *adminServiceProxyServer) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	if !s.access.IsAllowed("RebuildMutableState") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RebuildMutableState is not allowed.")
	}

	return s.adminClient.RebuildMutableState(ctx, in0)
}

func (s *adminServiceProxyServer) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	if !s.access.IsAllowed("RefreshWorkflowTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RefreshWorkflowTasks is not allowed.")
	}

	return s.adminClient.RefreshWorkflowTasks(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	if !s.access.IsAllowed("RemoveRemoteCluster") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveRemoteCluster is not allowed.")
	}

	return s.adminClient.RemoveRemoteCluster(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	if !s.access.IsAllowed("RemoveSearchAttributes") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveSearchAttributes is not allowed.")
	}

	return s.adminClient.RemoveSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	if !s.access.IsAllowed("RemoveTask") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveTask is not allowed.")
	}

	return s.adminClient.RemoveTask(ctx, in0)
}

func (s *adminServiceProxyServer) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	if !s.access.IsAllowed("ResendReplicationTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ResendReplicationTasks is not allowed.")
	}

	return s.adminClient.ResendReplicationTasks(ctx, in0)
}

func ClusterShardIDtoString(sd history.ClusterShardID) string {
	return fmt.Sprintf("(id: %d, shard: %d)", sd.ClusterID, sd.ShardID)
}

func (s *adminServiceProxyServer) StreamWorkflowReplicationMessages(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	if !s.access.IsAllowed("StreamWorkflowReplicationMessages") {
		return status.Errorf(codes.PermissionDenied, "Calling method StreamWorkflowReplicationMessages is not allowed.")
	}

	defer log.CapturePanic(s.logger, &retError)

	targetMetadata, ok := metadata.FromIncomingContext(targetStreamServer.Context())
	if !ok {
		return serviceerror.NewInvalidArgument("missing cluster & shard ID metadata")
	}
	targetClusterShardID, sourceClusterShardID, err := history.DecodeClusterShardMD(targetMetadata)
	if err != nil {
		return err
	}

	logger := log.With(s.logger,
		tag.NewStringTag("source", ClusterShardIDtoString(sourceClusterShardID)),
		tag.NewStringTag("target", ClusterShardIDtoString(targetClusterShardID)))

	logger.Info("AdminStreamReplicationMessages started.")
	defer logger.Info("AdminStreamReplicationMessages stopped.")

	// simply forwarding target metadata
	outgoingContext := metadata.NewOutgoingContext(targetStreamServer.Context(), targetMetadata)
	outgoingContext, cancel := context.WithCancel(outgoingContext)
	defer cancel()

	sourceStreamClient, err := s.adminClient.StreamWorkflowReplicationMessages(outgoingContext)
	if err != nil {
		logger.Error("remoteAdminServiceClient.StreamWorkflowReplicationMessages encountered error", tag.Error(err))
		return err
	}

	shutdownChan := channel.NewShutdownOnce()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer func() {
			shutdownChan.Shutdown()
			wg.Done()

			err = sourceStreamClient.CloseSend()
			if err != nil {
				logger.Error("Failed to close sourceStreamClient", tag.Error(err))
			}
		}()

		for !shutdownChan.IsShutdown() {
			req, err := targetStreamServer.Recv()
			if err == io.EOF {
				return
			}

			if err != nil {
				logger.Error("targetStreamServer.Recv encountered error", tag.Error(err))
				return
			}

			switch attr := req.GetAttributes().(type) {
			case *adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState:
				logger.Debug(fmt.Sprintf("forwarding SyncReplicationState: inclusive %v", attr.SyncReplicationState.InclusiveLowWatermark))

				if err = sourceStreamClient.Send(req); err != nil {
					logger.Error("sourceStreamClient.Send encountered error", tag.Error(err))
					return
				}
			default:
				logger.Error("targetStreamServer.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
					"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
				))))
				return
			}
		}
	}()
	go func() {
		defer func() {
			shutdownChan.Shutdown()
			wg.Done()
		}()

		for !shutdownChan.IsShutdown() {
			resp, err := sourceStreamClient.Recv()
			if err == io.EOF {
				return
			}

			if err != nil {
				logger.Error("sourceStreamClient.Recv encountered error", tag.Error(err))
				return
			}
			switch attr := resp.GetAttributes().(type) {
			case *adminservice.StreamWorkflowReplicationMessagesResponse_Messages:
				logger.Debug(fmt.Sprintf("forwarding ReplicationMessages: exclusive %v", attr.Messages.ExclusiveHighWatermark))
				if err = targetStreamServer.Send(resp); err != nil {
					if err != io.EOF {
						logger.Error("targetStreamServer.Send encountered error", tag.Error(err))

					}
					return
				}
			default:
				logger.Error("sourceStreamClient.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
					"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
				))))
				return
			}
		}
	}()

	wg.Wait()
	return nil
}
