package proxy

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/gogo/status"
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
	"github.com/temporalio/s2s-proxy/transport"
)

type (
	clusterInfo struct {
		serverAddress  string
		clusterShardID history.ClusterShardID
		s2sProxyConfig *config.S2SProxyConfig // if provided, used for setting up proxy
	}

	echoServer struct {
		server            *s2sproxy.TemporalAPIServer
		proxy             *s2sproxy.Proxy
		clusterInfo       clusterInfo
		remoteClusterInfo clusterInfo
		clientProvider    client.ClientProvider
		logger            log.Logger
	}

	watermarkInfo struct {
		Watermark int64
		Timestamp time.Time
	}
)

// Echo server for testing replication calls with or without proxies.
// It consists of 1/ a server for handling replication requests from remote server and 2/ a client for
// sending replication requests to remote server.
func newEchoServer(
	localClusterInfo clusterInfo,
	remoteClusterInfo clusterInfo,
	serviceName string,
	logger log.Logger,
	namespaces []string,
) *echoServer {
	ns := map[string]bool{}
	for _, n := range namespaces {
		ns[n] = true
	}
	// echoAdminService handles StreamWorkflowReplicationMessages call from remote server.
	// It acts as stream sender by echoing back InclusiveLowWatermark in SyncReplicationState message.
	senderAdminService := &echoAdminService{
		serviceName: serviceName,
		logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(localClusterInfo.serverAddress)),
		namespaces:  ns,
	}

	senderWorkflowService := &echoWorkflowService{
		serviceName: serviceName,
		logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(localClusterInfo.serverAddress)),
	}

	var proxy *s2sproxy.Proxy
	var err error
	var clientConfig config.ClientConfig

	localProxyCfg := localClusterInfo.s2sProxyConfig
	remoteProxyCfg := remoteClusterInfo.s2sProxyConfig

	if localProxyCfg != nil {
		// Setup local proxy ForwardAddress
		if remoteProxyCfg != nil {
			localProxyCfg.Outbound.Client.ServerAddress = remoteProxyCfg.Inbound.Server.ListenAddress
		} else {
			localProxyCfg.Outbound.Client.ServerAddress = remoteClusterInfo.serverAddress
		}

		configProvider := config.NewMockConfigProvider(*localClusterInfo.s2sProxyConfig)
		transportProvider, err := transport.NewTransprotProvider(configProvider)
		if err != nil {
			logger.Fatal("Failed to create transport provider", tag.Error(err))
		}

		clientFactory := client.NewClientFactory(transportProvider, logger)
		proxy, err = s2sproxy.NewProxy(
			configProvider,
			logger,
			clientFactory,
		)

		if err != nil {
			logger.Fatal("Failed to create proxy", tag.Error(err))
		}

		clientConfig = config.ClientConfig{
			TCPClientSetting: config.TCPClientSetting{
				ServerAddress: localProxyCfg.Outbound.Server.ListenAddress,
			},
		}
	} else {
		// No local proxy
		if remoteProxyCfg != nil {
			clientConfig = config.ClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: remoteProxyCfg.Inbound.Server.ListenAddress,
				},
			}
		} else {
			clientConfig = config.ClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: remoteClusterInfo.serverAddress,
				},
			}
		}
	}

	serverConfig := config.ServerConfig{
		TCPServerSetting: config.TCPServerSetting{
			ListenAddress: localClusterInfo.serverAddress,
		},
	}

	provider := &transport.TransportProvider{}
	serverTransport, err := provider.CreateServerTransport(serverConfig)
	if err != nil {
		panic(err)
	}

	tcpTransport, err := transport.NewTransprotProvider(&config.EmptyConfigProvider)
	if err != nil {
		logger.Fatal("Failed to create transport provider", tag.Error(err))
	}

	return &echoServer{
		server: s2sproxy.NewTemporalAPIServer(
			serviceName,
			serverConfig,
			senderAdminService,
			senderWorkflowService,
			nil,
			serverTransport,
			logger),
		proxy:             proxy,
		clusterInfo:       localClusterInfo,
		remoteClusterInfo: remoteClusterInfo,
		clientProvider:    client.NewClientProvider(clientConfig, client.NewClientFactory(tcpTransport, logger), logger),
		logger:            log.With(logger, common.ServiceTag(serviceName)),
	}
}

func (s *echoServer) start() {
	_ = s.server.Start()
	if s.proxy != nil {
		_ = s.proxy.Start()
	}
}

func (s *echoServer) stop() {
	if s.proxy != nil {
		s.proxy.Stop()
	}
	s.server.Stop()
}

const (
	retryInterval = 1 * time.Second // Interval between retries
)

func retry[T interface{}](f func() (T, error), maxRetries int, logger log.Logger) (T, error) {
	var err error
	var output T
	for i := 0; i < maxRetries; i++ {
		output, err = f()
		if err != nil {
			// Check if the error is a gRPC Unavailable error
			if status.Code(err) == codes.Unavailable {
				logger.Warn("Retry due to Unavailable error", tag.Error(err))
				time.Sleep(retryInterval)
				continue
			}

			return output, err
		}

		return output, nil
	}

	return output, fmt.Errorf("failed to call method after %d retries: %w", maxRetries, err)
}

func (s *echoServer) DescribeCluster(req *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	adminClient, err := s.clientProvider.GetAdminClient()
	if err != nil {
		return nil, err
	}

	return adminClient.DescribeCluster(context.Background(), req)
}

func (s *echoServer) DescribeMutableState(req *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	adminClient, err := s.clientProvider.GetAdminClient()
	if err != nil {
		return nil, err
	}

	return adminClient.DescribeMutableState(context.Background(), req)
}

// Method for testing replication stream.
//
// It starts a bi-directional stream by connecting to remote server (which acts as stream sender).
// It sends a sequence of numbers as SyncReplicationState message and then wait for remote server
// to reply.
func (s *echoServer) SendAndRecv(sequence []int64) (map[int64]bool, error) {
	echoed := make(map[int64]bool)
	metaData := history.EncodeClusterShardMD(s.clusterInfo.clusterShardID, s.remoteClusterInfo.clusterShardID)
	targetContext := metadata.NewOutgoingContext(context.TODO(), metaData)

	adminClient, err := s.clientProvider.GetAdminClient()
	if err != nil {
		return echoed, err
	}

	stream, err := retry(func() (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
		return adminClient.StreamWorkflowReplicationMessages(targetContext)
	}, 5, s.logger)
	if err != nil {
		return echoed, err
	}

	s.logger.Info("==== SendAndRecv starting ====")

	for _, waterMark := range sequence {
		highWatermarkInfo := &watermarkInfo{
			Watermark: waterMark,
		}

		req := &adminservice.StreamWorkflowReplicationMessagesRequest{
			Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
				SyncReplicationState: &replicationpb.SyncReplicationState{
					HighPriorityState: &replicationpb.ReplicationState{
						InclusiveLowWatermark:     highWatermarkInfo.Watermark,
						InclusiveLowWatermarkTime: timestamppb.New(highWatermarkInfo.Timestamp),
					},
				},
			}}

		if err = stream.Send(req); err != nil {
			return echoed, err
		}
	}

	for i := 0; i < len(sequence); i++ {
		resp, err := stream.Recv()
		if err != nil {
			return echoed, err
		}

		switch attr := resp.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			waterMark := attr.Messages.ExclusiveHighWatermark
			echoed[waterMark] = true
		default:
			return echoed, fmt.Errorf("sourceStreamClient.Recv encountered error")
		}
	}

	_ = stream.CloseSend()
	s.logger.Info("==== SendAndRecv completed ====")
	return echoed, nil
}

// Test workflowservice by making some request.
// Remote server echoes the Namespace field in the request as the WorkflowNamespace field in the response.
func (r *echoServer) PollActivityTaskQueue(req *workflowservice.PollActivityTaskQueueRequest) (*workflowservice.PollActivityTaskQueueResponse, error) {
	wfclient, err := r.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.PollActivityTaskQueue(context.Background(), req)
}
