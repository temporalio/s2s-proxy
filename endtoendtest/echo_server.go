package endtoendtest

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/gogo/status"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/endtoendtest/testservices"
	"github.com/temporalio/s2s-proxy/metrics"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

type (
	ClusterInfo struct {
		ServerAddress  string
		ClusterShardID history.ClusterShardID
		S2sProxyConfig *config.S2SProxyConfig // if provided, used for setting up Proxy
	}

	EchoServer struct {
		ServerConfig config.ProxyServerConfig
		ClientConfig config.ProxyClientConfig
		// provides EchoService directly
		Temporal *testservices.TemporalServerWithListen
		// connects to Temporal directly
		RemoteClient      *grpc.ClientConn
		Proxy             *s2sproxy.Proxy
		ClusterInfo       ClusterInfo
		RemoteClusterInfo ClusterInfo
		Logger            log.Logger
		EchoService       *testservices.EchoAdminService
	}

	WatermarkInfo struct {
		Watermark int64
		Timestamp time.Time
	}
)

const (
	defaultPayloadSize = 1024
)

// NewEchoServer creates an Echo Server for testing replication calls with or without proxies.
// It consists of 1/ a Server for handling replication requests from remote Server and 2/ a client for
// sending replication requests to remote Server.
func NewEchoServer(
	localClusterInfo ClusterInfo,
	remoteClusterInfo ClusterInfo,
	serviceName string,
	logger log.Logger,
	namespaces []string,
) *EchoServer {
	ns := map[string]bool{}
	for _, n := range namespaces {
		ns[n] = true
	}
	// EchoAdminService handles StreamWorkflowReplicationMessages call from remote Server.
	// It acts as stream sender by echoing back InclusiveLowWatermark in SyncReplicationState message.
	senderAdminService := &testservices.EchoAdminService{
		ServiceName: serviceName,
		Logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(localClusterInfo.ServerAddress)),
		Namespaces:  ns,
		PayloadSize: defaultPayloadSize,
	}

	senderWorkflowService := &testservices.EchoWorkflowService{
		ServiceName: serviceName,
		Logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(localClusterInfo.ServerAddress)),
	}

	var proxy *s2sproxy.Proxy
	var clientConfig config.ProxyClientConfig

	localProxyCfg := localClusterInfo.S2sProxyConfig
	remoteProxyCfg := remoteClusterInfo.S2sProxyConfig

	if localProxyCfg != nil && remoteProxyCfg != nil {
		if localProxyCfg.Outbound.Client.Type != remoteProxyCfg.Inbound.Server.Type {
			panic(fmt.Sprintf("local Proxy outbound client type: %s doesn't match with remote inbound Server type: %s",
				localProxyCfg.Outbound.Client.Type,
				remoteProxyCfg.Inbound.Server.Type,
			))
		}
	}

	if localProxyCfg != nil {
		if localProxyCfg.Outbound.Client.Type != config.MuxTransport {
			// Setup local Proxy forwarded Server address explicitly if not using multiplex transport.
			// If use multiplex transport, then outbound -> inbound uses established multiplex connection.
			if remoteProxyCfg != nil {
				localProxyCfg.Outbound.Client.ServerAddress = remoteProxyCfg.Inbound.Server.ListenAddress
			} else {
				localProxyCfg.Outbound.Client.ServerAddress = remoteClusterInfo.ServerAddress
			}
		}

		configProvider := config.NewMockConfigProvider(*localClusterInfo.S2sProxyConfig)
		proxy = s2sproxy.NewProxy(
			configProvider,
			nil,
			logger,
		)

		clientConfig = config.ProxyClientConfig{
			TCPClientSetting: config.TCPClientSetting{
				ServerAddress: localProxyCfg.Outbound.Server.ListenAddress,
			},
		}
	} else {
		// No local Proxy
		if remoteProxyCfg != nil {
			clientConfig = config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: remoteProxyCfg.Inbound.Server.ListenAddress,
				},
			}
		} else {
			clientConfig = config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: remoteClusterInfo.ServerAddress,
				},
			}
		}
	}

	serverConfig := config.ProxyServerConfig{
		TCPServerSetting: config.TCPServerSetting{
			ListenAddress: localClusterInfo.ServerAddress,
		},
	}

	logger = log.With(logger, common.ServiceTag(serviceName))
	var parsedTLSCfg *tls.Config
	clientTLS := config.FromClientTLSConfig(clientConfig.TLS)
	if clientTLS.IsEnabled() {
		var err error
		parsedTLSCfg, err = encryption.GetClientTLSConfig(clientTLS)
		if err != nil {
			panic(err)
		}
	}
	client, err := grpc.NewClient(clientConfig.ServerAddress,
		grpcutil.MakeDialOptions(parsedTLSCfg, metrics.GetStandardGRPCClientInterceptor("local"))...)
	if err != nil {
		panic(err)
	}
	return &EchoServer{
		ServerConfig: serverConfig,
		ClientConfig: clientConfig,
		Temporal: testservices.NewTemporalAPIServer(
			serviceName,
			senderAdminService,
			senderWorkflowService,
			nil,
			localClusterInfo.ServerAddress,
			logger),
		RemoteClient:      client,
		EchoService:       senderAdminService,
		Proxy:             proxy,
		ClusterInfo:       localClusterInfo,
		RemoteClusterInfo: remoteClusterInfo,
		Logger:            logger,
	}
}

func (s *EchoServer) SetPayloadSize(size int) {
	s.EchoService.PayloadSize = size
}

func (s *EchoServer) Start() {
	s.Logger.Info(fmt.Sprintf("Starting echoServer with ServerConfig: %v, ClientConfig: %v", s.ServerConfig, s.ClientConfig))
	s.Temporal.Start()
	if s.Proxy != nil {
		_ = s.Proxy.Start()
	}
}

func (s *EchoServer) Stop() {
	if s.Proxy != nil {
		s.Proxy.Stop()
	}
	s.Temporal.Stop()
}

const (
	retryInterval = 200 * time.Millisecond // Interval between retries
)

func Retry[T interface{}](f func() (T, error), maxRetries int, logger log.Logger) (T, error) {
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

func (s *EchoServer) DescribeCluster(req *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	if s.RemoteClient == nil {
		panic("nil RemoteClient")
	}
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return adminservice.NewAdminServiceClient(s.RemoteClient).DescribeCluster(timeout, req)
}

func (s *EchoServer) DescribeMutableState(req *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return adminservice.NewAdminServiceClient(s.RemoteClient).DescribeMutableState(timeout, req)
}

func (s *EchoServer) CreateStreamClient() (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
	metaData := history.EncodeClusterShardMD(s.ClusterInfo.ClusterShardID, s.RemoteClusterInfo.ClusterShardID)
	targetContext := metadata.NewOutgoingContext(context.TODO(), metaData)

	adminClient := adminservice.NewAdminServiceClient(s.RemoteClient)

	return Retry(func() (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
		return adminClient.StreamWorkflowReplicationMessages(targetContext)
	}, 5, s.Logger)
}

func SendRecv(stream adminservice.AdminService_StreamWorkflowReplicationMessagesClient, sequence []int64) (map[int64]bool, error) {
	echoed := make(map[int64]bool)
	var err error
	for _, waterMark := range sequence {
		highWatermarkInfo := &WatermarkInfo{
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

	return echoed, nil
}

// Method for testing replication stream.
//
// It starts a bi-directional stream by connecting to remote Server (which acts as stream sender).
// It sends a sequence of numbers as SyncReplicationState message and then wait for remote Server
// to reply.
func (s *EchoServer) SendAndRecv(sequence []int64) (map[int64]bool, error) {
	echoed := make(map[int64]bool)
	stream, err := s.CreateStreamClient()
	if err != nil {
		return echoed, err
	}

	s.Logger.Info("==== SendAndRecv starting ====")
	echoed, err = SendRecv(stream, sequence)
	if err != nil {
		s.Logger.Error("SendRecv", tag.NewErrorTag("error", err))
	}
	_ = stream.CloseSend()
	s.Logger.Info("==== SendAndRecv completed ====")
	return echoed, err
}

// Test workflowservice by making some request.
// Remote Server echoes the Namespace field in the request as the WorkflowNamespace field in the response.
func (s *EchoServer) PollActivityTaskQueue(req *workflowservice.PollActivityTaskQueueRequest) (*workflowservice.PollActivityTaskQueueResponse, error) {
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return workflowservice.NewWorkflowServiceClient(s.RemoteClient).PollActivityTaskQueue(timeout, req)
}
func (s *EchoServer) Describe() string {
	proxyDescription := "no proxy"
	if s.Proxy != nil {
		proxyDescription = s.Proxy.Describe()
	}
	return fmt.Sprintf("[EchoServer \n"+
		"ClusterInfo: %v\n"+
		"RemoteClusterInfo: %v\n,"+
		"Server config: %v\n,"+
		"Client config: %v\n,"+
		"Proxy config: %v\n,"+
		"]", s.ServerConfig, s.ClientConfig, proxyDescription, s.ClusterInfo, s.RemoteClusterInfo)
}
