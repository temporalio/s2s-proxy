package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

type (
	// simpleGRPCServer is a self-sufficient package of listener, server, logger, and lifetime. It can be Started,
	// which creates a GoRoutine that listens on the provided listener using server.Serve until lifetime closes.
	simpleGRPCServer struct {
		name     string
		lifetime context.Context
		listener net.Listener
		server   *grpc.Server
		logger   log.Logger
	}

	// describableClientConn is a small extension to add a Describe() method to grpc.ClientConn. Used for logging.
	describableClientConn struct {
		*grpc.ClientConn
	}

	// ClusterConnection contains a bidirectional connection between a local Temporal server and a remote.
	ClusterConnection struct {
		// When lifetime closes, all clients and servers in ClusterConnection will stop.
		lifetime context.Context
		// outboundServer receives connections from the local Temporal and makes calls using outboundClient.
		outboundServer contextAwareServer
		// outboundClient is connected to a remote Temporal server somewhere.
		outboundClient closableClientConn
		// inboundServer receives connections from a remote Temporal server and calls using the inboundClient.
		inboundServer contextAwareServer
		// inboundClient talks to the local Temporal frontend.
		inboundClient    closableClientConn
		inboundObserver  *ReplicationStreamObserver
		outboundObserver *ReplicationStreamObserver
		shardManager     ShardManager
		intraMgr         *intraProxyManager
		logger           log.Logger

		// remoteSendChannels maps shard IDs to send channels for replication message routing
		remoteSendChannels   map[history.ClusterShardID]chan RoutedMessage
		remoteSendChannelsMu sync.RWMutex

		// localAckChannels maps shard IDs to ack channels for local acknowledgment handling
		localAckChannels   map[history.ClusterShardID]chan RoutedAck
		localAckChannelsMu sync.RWMutex

		// localReceiverCancelFuncs maps shard IDs to context cancel functions for local receiver termination
		localReceiverCancelFuncs   map[history.ClusterShardID]context.CancelFunc
		localReceiverCancelFuncsMu sync.RWMutex
	}
	// contextAwareServer represents a startable gRPC server used to provide the Temporal interface on some connection.
	// IsUsable and Describe allow the caller to know and log the current state of the server.
	contextAwareServer interface {
		Start()
		Describe() string
		Name() string
		CanAcceptConnections() bool
	}
	// closableClientConn represents a ClientConnInterface with a Close and a Describe. It's implemented by
	// grpcutil.MultiClientConn and describableClientConn.
	closableClientConn interface {
		grpc.ClientConnInterface
		Close() error
		Describe() string
		CanMakeCalls() bool
	}
	serverConfiguration struct {
		// name identifies the connection in metrics and in the mux custom resolver. Must be sanitized.
		name string
		// clusterDefinition contains the connection information for the server
		clusterDefinition config.ClusterDefinition
		// directionLabel is used in metrics. DO NOT hang application logic off of this! Pass the appropriate arguments instead.
		directionLabel string
		// client is the ClientConnInterface that the server will use to make calls.
		client closableClientConn
		// managedClient is updated by the multi-mux-manager that also owns the server. Needs some more cleanup.
		managedClient closableClientConn
		// nsTranslations and saTranslations are used to translate namespace and search attribute names.
		nsTranslations   collect.StaticBiMap[string, string]
		saTranslations   config.SearchAttributeTranslation
		shardCountConfig config.ShardCountConfig
		logger           log.Logger

		clusterConnection *ClusterConnection
		lcmParameters     LCMParameters
		routingParameters RoutingParameters
	}
)

func sanitizeConnectionName(name string) string {
	// Prometheus is more restrictive than grpc.Dial, so we'll just reuse the prometheus sanitizer for now
	return metrics.SanitizeForPrometheus(name)
}

// NewClusterConnection unpacks the connConfig and creates the inbound and outbound clients and servers.
func NewClusterConnection(lifetime context.Context, connConfig config.ClusterConnConfig, logger log.Logger) (*ClusterConnection, error) {
	// The name is used in metrics and in the protocol for identifying the multi-client-conn. Sanitize it or else grpc.Dial will be very unhappy.
	sanitizedConnectionName := sanitizeConnectionName(connConfig.Name)
	cc := &ClusterConnection{
		lifetime:                 lifetime,
		logger:                   log.With(logger, tag.NewStringTag("clusterConn", sanitizedConnectionName)),
		remoteSendChannels:       make(map[history.ClusterShardID]chan RoutedMessage),
		localAckChannels:         make(map[history.ClusterShardID]chan RoutedAck),
		localReceiverCancelFuncs: make(map[history.ClusterShardID]context.CancelFunc),
	}
	var err error
	cc.inboundClient, err = createClient(lifetime, sanitizedConnectionName, connConfig.LocalServer.Connection, "inbound")
	if err != nil {
		return nil, err
	}
	cc.outboundClient, err = createClient(lifetime, sanitizedConnectionName, connConfig.RemoteServer.Connection, "outbound")
	if err != nil {
		return nil, err
	}
	nsTranslations, err := connConfig.NamespaceTranslation.AsLocalToRemoteBiMap()
	if err != nil {
		return nil, err
	}
	saTranslations, err := connConfig.SearchAttributeTranslation.AsLocalToRemoteSATranslation()
	if err != nil {
		return nil, err
	}

	getLCMParameters := func(shardCountConfig config.ShardCountConfig, inverse bool) LCMParameters {
		if shardCountConfig.Mode != config.ShardCountLCM {
			return LCMParameters{}
		}
		lcm := common.LCM(shardCountConfig.LocalShardCount, shardCountConfig.RemoteShardCount)
		if inverse {
			return LCMParameters{
				LCM:              lcm,
				TargetShardCount: shardCountConfig.LocalShardCount,
			}
		}
		return LCMParameters{
			LCM:              lcm,
			TargetShardCount: shardCountConfig.RemoteShardCount,
		}
	}
	getRoutingParameters := func(shardCountConfig config.ShardCountConfig, inverse bool, directionLabel string) RoutingParameters {
		if shardCountConfig.Mode != config.ShardCountRouting {
			return RoutingParameters{}
		}
		if inverse {
			return RoutingParameters{
				OverrideShardCount:     shardCountConfig.RemoteShardCount,
				RoutingLocalShardCount: shardCountConfig.LocalShardCount,
				DirectionLabel:         directionLabel,
			}
		}
		return RoutingParameters{
			OverrideShardCount:     shardCountConfig.LocalShardCount,
			RoutingLocalShardCount: shardCountConfig.RemoteShardCount,
			DirectionLabel:         directionLabel,
		}
	}

	cc.inboundServer, cc.inboundObserver, err = createServer(lifetime, serverConfiguration{
		name:              sanitizedConnectionName,
		clusterDefinition: connConfig.RemoteServer,
		directionLabel:    "inbound",
		client:            cc.inboundClient,
		managedClient:     cc.outboundClient,
		nsTranslations:    nsTranslations.Inverse(),
		saTranslations:    saTranslations.Inverse(),
		shardCountConfig:  connConfig.ShardCountConfig,
		logger:            cc.logger,
		clusterConnection: cc,
		lcmParameters:     getLCMParameters(connConfig.ShardCountConfig, true),
		routingParameters: getRoutingParameters(connConfig.ShardCountConfig, true, "inbound"),
	})
	if err != nil {
		return nil, err
	}

	cc.outboundServer, cc.outboundObserver, err = createServer(lifetime, serverConfiguration{
		name:              sanitizedConnectionName,
		clusterDefinition: connConfig.LocalServer,
		directionLabel:    "outbound",
		client:            cc.outboundClient,
		managedClient:     cc.inboundClient,
		nsTranslations:    nsTranslations,
		saTranslations:    saTranslations,
		shardCountConfig:  connConfig.ShardCountConfig,
		logger:            cc.logger,
		clusterConnection: cc,
		lcmParameters:     getLCMParameters(connConfig.ShardCountConfig, false),
		routingParameters: getRoutingParameters(connConfig.ShardCountConfig, false, "outbound"),
	})
	if err != nil {
		return nil, err
	}

	cc.shardManager = NewShardManager(connConfig.MemberlistConfig, logger)
	if connConfig.MemberlistConfig != nil {
		cc.intraMgr = newIntraProxyManager(logger, cc, connConfig.ShardCountConfig)
	}

	return cc, nil
}

func createClient(lifetime context.Context, connectionName string, transportCfg config.TransportInfo, directionLabel string) (closableClientConn, error) {
	switch transportCfg.ConnectionType {
	case config.ConnTypeTCP:
		return buildTLSTCPClient(lifetime, transportCfg.TcpClient.ConnectionString, transportCfg.TcpClient.TLSConfig, directionLabel)
	case config.ConnTypeMuxClient, config.ConnTypeMuxServer:
		return grpcutil.NewMultiClientConn(lifetime, fmt.Sprintf("client-conn-%s", connectionName),
			// TLS is handled by the mux connection, so tlsConfig will always be nil
			grpcutil.MakeDialOptions(nil, metrics.GetGRPCClientMetrics(directionLabel))...)
	default:
		return nil, errors.New("invalid connection type")
	}
}

func createServer(lifetime context.Context, c serverConfiguration) (contextAwareServer, *ReplicationStreamObserver, error) {
	switch c.clusterDefinition.Connection.ConnectionType {
	case config.ConnTypeTCP:
		// No special logic required for managedClient
		return createTCPServer(lifetime, c)
	case config.ConnTypeMuxClient, config.ConnTypeMuxServer:
		observer := NewReplicationStreamObserver(c.logger)
		grpcServer, err := buildProxyServer(c, c.clusterDefinition.Connection.MuxAddressInfo.TLSConfig, observer.ReportStreamValue)
		if err != nil {
			return nil, nil, err
		}
		// The Mux manager needs to update its associated client
		muxMgr, err := mux.NewGRPCMuxManager(lifetime, c.name, c.clusterDefinition, c.managedClient.(*grpcutil.MultiClientConn), grpcServer, c.logger)
		if err != nil {
			return nil, nil, err
		}
		return muxMgr, observer, nil
	default:
		return nil, nil, errors.New("invalid connection type")
	}
}

func createTCPServer(lifetime context.Context, c serverConfiguration) (contextAwareServer, *ReplicationStreamObserver, error) {
	observer := NewReplicationStreamObserver(c.logger)
	listener, err := net.Listen("tcp", c.clusterDefinition.Connection.TcpServer.ConnectionString)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid configuration for inbound server: %w", err)
	}
	grpcServer, err := buildProxyServer(c, c.clusterDefinition.Connection.TcpServer.TLSConfig, observer.ReportStreamValue)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create inbound server: %w", err)
	}
	server := &simpleGRPCServer{
		name:     c.name,
		lifetime: lifetime,
		listener: listener,
		server:   grpcServer,
		logger:   c.logger,
	}
	return server, observer, nil
}

// buildTLSTCPClient creates a grpc.ClientConn using the provided configuration. It schedules a goroutine that closes
// the grpc.ClientConn when the provided lifetime ends.
func buildTLSTCPClient(lifetime context.Context, serverAddress string, tlsCfg encryption.TLSConfig, metricLabel string) (closableClientConn, error) {
	var parsedTLSCfg *tls.Config
	if tlsCfg.IsEnabled() {
		var err error
		parsedTLSCfg, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("config error when creating tls config: %w", err)
		}
	}
	client, err := grpc.NewClient(serverAddress, grpcutil.MakeDialOptions(parsedTLSCfg, metrics.GetGRPCClientMetrics(metricLabel))...)
	if err != nil {
		return nil, fmt.Errorf("could not create inbound client: %w", err)
	}
	context.AfterFunc(lifetime, func() {
		// grpc.ClientConn must be closed, but it's not context-aware. Make sure the client closes when the lifetime ends
		_ = client.Close()
	})
	return describableClientConn{client}, nil
}

func (c *ClusterConnection) Start() {
	if c.shardManager != nil {
		err := c.shardManager.Start(c.lifetime)
		if err != nil {
			c.logger.Error("Failed to start shard manager", tag.Error(err))
		}
	}
	if c.intraMgr != nil {
		c.intraMgr.Start()
	}
	c.inboundServer.Start()
	c.inboundObserver.Start(c.lifetime, c.inboundServer.Name(), "inbound")
	c.outboundServer.Start()
	c.outboundObserver.Start(c.lifetime, c.outboundServer.Name(), "outbound")
}

func (c *ClusterConnection) Describe() string {
	return fmt.Sprintf("[ClusterConnection connects outbound server %s to outbound client %s, inbound server %s to inbound client %s]",
		c.outboundServer.Describe(), c.outboundClient.Describe(), c.inboundServer.Describe(), c.inboundClient.Describe())
}

func (c *ClusterConnection) AcceptingInboundTraffic() bool {
	return c.inboundClient.CanMakeCalls() && c.inboundServer.CanAcceptConnections()
}

func (c *ClusterConnection) AcceptingOutboundTraffic() bool {
	return c.outboundClient.CanMakeCalls() && c.outboundServer.CanAcceptConnections()
}

// GetShardInfo returns debug information about shard distribution
func (c *ClusterConnection) GetShardInfos() []ShardDebugInfo {
	var shardInfos []ShardDebugInfo
	if c.shardManager != nil {
		shardInfos = append(shardInfos, c.shardManager.GetShardInfo())
	}
	return shardInfos
}

// GetChannelInfo returns debug information about active channels
func (c *ClusterConnection) GetChannelInfo() ChannelDebugInfo {
	remoteSendChannels := make(map[string]int)
	var totalSendChannels int

	// Collect remote send channel info first
	c.remoteSendChannelsMu.RLock()
	for shardID, ch := range c.remoteSendChannels {
		shardKey := ClusterShardIDtoString(shardID)
		remoteSendChannels[shardKey] = len(ch)
	}
	totalSendChannels = len(c.remoteSendChannels)
	c.remoteSendChannelsMu.RUnlock()

	localAckChannels := make(map[string]int)
	var totalAckChannels int

	// Collect local ack channel info separately
	c.localAckChannelsMu.RLock()
	for shardID, ch := range c.localAckChannels {
		shardKey := ClusterShardIDtoString(shardID)
		localAckChannels[shardKey] = len(ch)
	}
	totalAckChannels = len(c.localAckChannels)
	c.localAckChannelsMu.RUnlock()

	return ChannelDebugInfo{
		RemoteSendChannels: remoteSendChannels,
		LocalAckChannels:   localAckChannels,
		TotalSendChannels:  totalSendChannels,
		TotalAckChannels:   totalAckChannels,
	}
}

// SetRemoteSendChan registers a send channel for a specific shard ID
func (c *ClusterConnection) SetRemoteSendChan(shardID history.ClusterShardID, sendChan chan RoutedMessage) {
	c.logger.Info("Register remote send channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	c.remoteSendChannelsMu.Lock()
	defer c.remoteSendChannelsMu.Unlock()
	c.remoteSendChannels[shardID] = sendChan
}

// GetRemoteSendChan retrieves the send channel for a specific shard ID
func (c *ClusterConnection) GetRemoteSendChan(shardID history.ClusterShardID) (chan RoutedMessage, bool) {
	c.remoteSendChannelsMu.RLock()
	defer c.remoteSendChannelsMu.RUnlock()
	ch, exists := c.remoteSendChannels[shardID]
	return ch, exists
}

// GetAllRemoteSendChans returns a map of all remote send channels
func (c *ClusterConnection) GetAllRemoteSendChans() map[history.ClusterShardID]chan RoutedMessage {
	c.remoteSendChannelsMu.RLock()
	defer c.remoteSendChannelsMu.RUnlock()

	// Create a copy of the map
	result := make(map[history.ClusterShardID]chan RoutedMessage, len(c.remoteSendChannels))
	for k, v := range c.remoteSendChannels {
		result[k] = v
	}
	return result
}

// GetRemoteSendChansByCluster returns a copy of remote send channels filtered by clusterID
func (c *ClusterConnection) GetRemoteSendChansByCluster(clusterID int32) map[history.ClusterShardID]chan RoutedMessage {
	c.remoteSendChannelsMu.RLock()
	defer c.remoteSendChannelsMu.RUnlock()

	result := make(map[history.ClusterShardID]chan RoutedMessage)
	for k, v := range c.remoteSendChannels {
		if k.ClusterID == clusterID {
			result[k] = v
		}
	}
	return result
}

// RemoveRemoteSendChan removes the send channel for a specific shard ID only if it matches the provided channel
func (c *ClusterConnection) RemoveRemoteSendChan(shardID history.ClusterShardID, expectedChan chan RoutedMessage) {
	c.remoteSendChannelsMu.Lock()
	defer c.remoteSendChannelsMu.Unlock()
	if currentChan, exists := c.remoteSendChannels[shardID]; exists && currentChan == expectedChan {
		delete(c.remoteSendChannels, shardID)
		c.logger.Info("Removed remote send channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	} else {
		c.logger.Info("Skipped removing remote send channel for shard (channel mismatch or already removed)", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	}
}

// SetLocalAckChan registers an ack channel for a specific shard ID
func (c *ClusterConnection) SetLocalAckChan(shardID history.ClusterShardID, ackChan chan RoutedAck) {
	c.logger.Info("Register local ack channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	c.localAckChannelsMu.Lock()
	defer c.localAckChannelsMu.Unlock()
	c.localAckChannels[shardID] = ackChan
}

// GetLocalAckChan retrieves the ack channel for a specific shard ID
func (c *ClusterConnection) GetLocalAckChan(shardID history.ClusterShardID) (chan RoutedAck, bool) {
	c.localAckChannelsMu.RLock()
	defer c.localAckChannelsMu.RUnlock()
	ch, exists := c.localAckChannels[shardID]
	return ch, exists
}

// RemoveLocalAckChan removes the ack channel for a specific shard ID only if it matches the provided channel
func (c *ClusterConnection) RemoveLocalAckChan(shardID history.ClusterShardID, expectedChan chan RoutedAck) {
	c.logger.Info("Remove local ack channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	c.localAckChannelsMu.Lock()
	defer c.localAckChannelsMu.Unlock()
	if currentChan, exists := c.localAckChannels[shardID]; exists && currentChan == expectedChan {
		delete(c.localAckChannels, shardID)
	} else {
		c.logger.Info("Skipped removing local ack channel for shard (channel mismatch or already removed)", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	}
}

// ForceRemoveLocalAckChan unconditionally removes the ack channel for a specific shard ID
func (c *ClusterConnection) ForceRemoveLocalAckChan(shardID history.ClusterShardID) {
	c.logger.Info("Force remove local ack channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	c.localAckChannelsMu.Lock()
	defer c.localAckChannelsMu.Unlock()
	delete(c.localAckChannels, shardID)
}

// SetLocalReceiverCancelFunc registers a cancel function for a local receiver for a specific shard ID
func (c *ClusterConnection) SetLocalReceiverCancelFunc(shardID history.ClusterShardID, cancelFunc context.CancelFunc) {
	c.logger.Info("Register local receiver cancel function for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	c.localReceiverCancelFuncsMu.Lock()
	defer c.localReceiverCancelFuncsMu.Unlock()
	c.localReceiverCancelFuncs[shardID] = cancelFunc
}

// GetLocalReceiverCancelFunc retrieves the cancel function for a local receiver for a specific shard ID
func (c *ClusterConnection) GetLocalReceiverCancelFunc(shardID history.ClusterShardID) (context.CancelFunc, bool) {
	c.localReceiverCancelFuncsMu.RLock()
	defer c.localReceiverCancelFuncsMu.RUnlock()
	cancelFunc, exists := c.localReceiverCancelFuncs[shardID]
	return cancelFunc, exists
}

// RemoveLocalReceiverCancelFunc unconditionally removes the cancel function for a local receiver for a specific shard ID
// Note: Functions cannot be compared in Go, so we use unconditional removal.
// The race condition is primarily with channels; TerminatePreviousLocalReceiver handles forced cleanup.
func (c *ClusterConnection) RemoveLocalReceiverCancelFunc(shardID history.ClusterShardID) {
	c.logger.Info("Remove local receiver cancel function for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
	c.localReceiverCancelFuncsMu.Lock()
	defer c.localReceiverCancelFuncsMu.Unlock()
	delete(c.localReceiverCancelFuncs, shardID)
}

// TerminatePreviousLocalReceiver checks if there is a previous local receiver for this shard and terminates it if needed
func (c *ClusterConnection) TerminatePreviousLocalReceiver(shardID history.ClusterShardID) {
	// Check if there's a previous cancel function for this shard
	if prevCancelFunc, exists := c.GetLocalReceiverCancelFunc(shardID); exists {
		c.logger.Info("Terminating previous local receiver for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))

		// Cancel the previous receiver's context
		prevCancelFunc()

		// Force remove the cancel function and ack channel from tracking
		c.RemoveLocalReceiverCancelFunc(shardID)
		c.ForceRemoveLocalAckChan(shardID)
	}
}

// buildProxyServer uses the provided grpc.ClientConnInterface and config.ProxyConfig to create a grpc.Server that proxies
// the Temporal API across the ClientConnInterface.
func buildProxyServer(c serverConfiguration, tlsConfig encryption.TLSConfig, observeFn func(int32, int32)) (*grpc.Server, error) {
	serverOpts, err := makeServerOptions(c, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("could not parse server options: %w", err)
	}
	server := grpc.NewServer(serverOpts...)

	adminServiceImpl := NewAdminServiceProxyServer(
		fmt.Sprintf("%sAdminService", c.directionLabel),
		adminservice.NewAdminServiceClient(c.client),
		adminservice.NewAdminServiceClient(c.managedClient),
		c.clusterDefinition.APIOverrides,
		[]string{c.directionLabel},
		observeFn,
		c.shardCountConfig,
		c.lcmParameters,
		c.routingParameters,
		c.logger,
		c.clusterConnection,
	)
	var accessControl *auth.AccessControl
	if c.clusterDefinition.ACLPolicy != nil {
		accessControl = auth.NewAccesControl(c.clusterDefinition.ACLPolicy.AllowedNamespaces)
	}
	workflowServiceImpl := NewWorkflowServiceProxyServer("inboundWorkflowService", workflowservice.NewWorkflowServiceClient(c.client),
		accessControl, c.logger)
	adminservice.RegisterAdminServiceServer(server, adminServiceImpl)
	workflowservice.RegisterWorkflowServiceServer(server, workflowServiceImpl)
	return server, nil
}

func makeServerOptions(c serverConfiguration, tlsConfig encryption.TLSConfig) ([]grpc.ServerOption, error) {
	var unaryInterceptors []grpc.UnaryServerInterceptor
	var streamInterceptors []grpc.StreamServerInterceptor
	labelGenerator := grpcprom.WithLabelsFromContext(func(_ context.Context) (labels prometheus.Labels) {
		return prometheus.Labels{"direction": c.directionLabel}
	})

	// Ordering matters! These metrics happen BEFORE the translations/acl
	unaryInterceptors = append(unaryInterceptors, metrics.GRPCServerMetrics.UnaryServerInterceptor(labelGenerator))
	streamInterceptors = append(streamInterceptors, metrics.GRPCServerMetrics.StreamServerInterceptor(labelGenerator))

	var translators []interceptor.Translator
	if c.nsTranslations.Len() > 0 {
		translators = append(translators, interceptor.NewNamespaceNameTranslator(c.logger, c.nsTranslations.AsMap(), c.nsTranslations.Inverse().AsMap()))
	}

	if c.saTranslations.LenNamespaces() > 0 {
		c.logger.Info("search attribute translation enabled", tag.NewAnyTag("mappings", c.saTranslations))
		if c.saTranslations.LenNamespaces() > 1 {
			panic("multiple namespace search attribute mappings are not supported")
		}
		translators = append(translators, interceptor.NewSearchAttributeTranslator(c.logger, c.saTranslations.FlattenMaps(), c.saTranslations.Inverse().FlattenMaps()))
	}

	if len(translators) > 0 {
		tr := interceptor.NewTranslationInterceptor(c.logger, translators)
		unaryInterceptors = append(unaryInterceptors, tr.Intercept)
		streamInterceptors = append(streamInterceptors, tr.InterceptStream)
	}

	if c.clusterDefinition.ACLPolicy != nil {
		aclInterceptor := interceptor.NewAccessControlInterceptor(c.logger, c.clusterDefinition.ACLPolicy.AllowedMethods.AdminService, c.clusterDefinition.ACLPolicy.AllowedNamespaces)
		unaryInterceptors = append(unaryInterceptors, aclInterceptor.Intercept)
		streamInterceptors = append(streamInterceptors, aclInterceptor.StreamIntercept)
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	}

	if tlsConfig.IsEnabled() {
		tlsConfig, err := encryption.GetServerTLSConfig(tlsConfig, c.logger)
		if err != nil {
			return opts, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	return opts, nil
}

func (s *simpleGRPCServer) Start() {
	metrics.GRPCServerStarted.WithLabelValues(s.name).Inc()
	go func() {
		s.logger.Info("Starting TCP-TLS gRPC server", tag.Name(s.name), tag.Address(s.listener.Addr().String()))
		for s.lifetime.Err() == nil {
			err := s.server.Serve(s.listener)
			if s.lifetime.Err() != nil {
				// Cluster is closing, just exit.
				return
			}
			if err != nil {
				s.logger.Warn("GRPC server failed", tag.NewStringTag("direction", "outbound"), tag.Address(s.listener.Addr().String()), tag.Error(err))
				if err == io.EOF {
					metrics.GRPCServerStopped.WithLabelValues(s.name, "eof").Inc()
				} else if !errors.Is(err, grpc.ErrServerStopped) {
					metrics.GRPCServerStopped.WithLabelValues(s.name, "unknown").Inc()
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()
	// The basic net.Listen, grpc.Server, and ClientConn are not context-aware, so make sure they clean up on context close.
	context.AfterFunc(s.lifetime, func() {
		metrics.GRPCServerStopped.WithLabelValues(s.name, "none").Inc()
		s.server.GracefulStop()
		_ = s.listener.Close()
	})
}
func (s *simpleGRPCServer) CanAcceptConnections() bool {
	return true
}
func (s *simpleGRPCServer) CanMakeCalls() bool {
	return true
}
func (s *simpleGRPCServer) Describe() string {
	return fmt.Sprintf("[simpleGRPCServer %s listening on %s. lifetime.Err: %e]", s.name, s.listener.Addr(), s.lifetime.Err())
}
func (s *simpleGRPCServer) Name() string {
	return s.name
}
func (d describableClientConn) Describe() string {
	return fmt.Sprintf("[grpc.ClientConn %s, state=%s]", d.Target(), d.GetState().String())
}
func (d describableClientConn) CanMakeCalls() bool {
	return true
}
