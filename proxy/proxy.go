package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	migrationId struct {
		name string
		// Needs some config revision before uncommenting:
		//accountId string
	}

	// RoutedAck wraps an ACK with the target shard it originated from
	RoutedAck struct {
		TargetShard history.ClusterShardID
		Req         *adminservice.StreamWorkflowReplicationMessagesRequest
	}

	// RoutedMessage wraps a replication response with originating client shard info
	RoutedMessage struct {
		SourceShard history.ClusterShardID
		Resp        *adminservice.StreamWorkflowReplicationMessagesResponse
	}

	Proxy struct {
		lifetime                  context.Context
		cancel                    context.CancelFunc
		inboundHealthCheckConfig  *config.HealthCheckConfig
		outboundHealthCheckConfig *config.HealthCheckConfig
		metricsConfig             *config.MetricsConfig
		clusterConnections        map[migrationId]*ClusterConnection
		inboundHealthCheckServer  *http.Server
		outboundHealthCheckServer *http.Server
		metricsServer             *http.Server
		shardManager              ShardManager
		logger                    log.Logger

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
)

func NewProxy(configProvider config.ConfigProvider, shardManager ShardManager, logger log.Logger) *Proxy {
	s2sConfig := config.ToClusterConnConfig(configProvider.GetS2SProxyConfig())
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &Proxy{
		lifetime:           ctx,
		cancel:             cancel,
		clusterConnections: make(map[migrationId]*ClusterConnection, len(s2sConfig.MuxTransports)),
		shardManager:       shardManager,
		logger: log.NewThrottledLogger(
			logger,
			func() float64 {
				return s2sConfig.Logging.GetThrottleMaxRPS()
			},
		),
		remoteSendChannels:       make(map[history.ClusterShardID]chan RoutedMessage),
		localAckChannels:         make(map[history.ClusterShardID]chan RoutedAck),
		localReceiverCancelFuncs: make(map[history.ClusterShardID]context.CancelFunc),
	}
	if len(s2sConfig.ClusterConnections) == 0 {
		panic(errors.New("cannot create proxy without inbound and outbound config"))
	}
	if s2sConfig.Metrics != nil {
		proxy.metricsConfig = s2sConfig.Metrics
	}
	for _, clusterCfg := range s2sConfig.ClusterConnections {
		cc, err := NewClusterConnection(ctx, clusterCfg, shardManager, logger)
		if err != nil {
			logger.Fatal("Incorrectly configured Mux cluster connection", tag.Error(err), tag.NewStringTag("name", clusterCfg.Name))
			continue
		}
		proxy.clusterConnections[migrationId{clusterCfg.Name}] = cc
	}
	// TODO: correctly host multiple health checks
	if len(s2sConfig.ClusterConnections) > 0 && s2sConfig.ClusterConnections[0].InboundHealthCheck.ListenAddress != "" {
		proxy.inboundHealthCheckConfig = &s2sConfig.ClusterConnections[0].InboundHealthCheck
	}
	if len(s2sConfig.ClusterConnections) > 0 && s2sConfig.ClusterConnections[0].OutboundHealthCheck.ListenAddress != "" {
		proxy.outboundHealthCheckConfig = &s2sConfig.ClusterConnections[0].OutboundHealthCheck
	}

	metrics.NewProxyCount.Inc()
	return proxy
}

func (s *Proxy) startHealthCheckHandler(lifetime context.Context, healthChecker HealthChecker, cfg config.HealthCheckConfig) (*http.Server, error) {
	if cfg.Protocol != config.HTTP {
		return nil, fmt.Errorf("unsupported health check protocol %s", cfg.Protocol)
	}

	// Set up the handler. Avoid the global ServeMux so that we can create N of these in unit test suites
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/health", healthChecker.createHandler())
	// Define the server and its settings
	healthCheckServer := &http.Server{
		Addr:    cfg.ListenAddress,
		Handler: serveMux,
	}

	go func() {
		s.logger.Info("Starting health check server", tag.Address(cfg.ListenAddress))
		if err := healthCheckServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Error starting server", tag.Error(err))
		}
	}()
	context.AfterFunc(lifetime, func() {
		_ = healthCheckServer.Close()
	})
	return healthCheckServer, nil
}

func (s *Proxy) startMetricsHandler(lifetime context.Context, cfg config.MetricsConfig) error {
	// Why not use the global ServeMux? So that it can be used in unit tests
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.NewMetricsHandler(s.logger))
	s.metricsServer = &http.Server{
		Addr:    cfg.Prometheus.ListenAddress,
		Handler: mux,
	}

	go func() {
		s.logger.Info("Starting metrics server", tag.Address(cfg.Prometheus.ListenAddress))
		if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Error starting server", tag.Error(err))
		}
	}()
	context.AfterFunc(lifetime, func() {
		_ = s.metricsServer.Close()
	})
	return nil
}

func (s *Proxy) Start() error {
	if s.inboundHealthCheckConfig != nil {
		var err error
		healthFn := func() bool {
			// TODO: overly conservative right now. When there are multiple remotes, we should track their health separately
			for _, cc := range s.clusterConnections {
				if !cc.AcceptingInboundTraffic() {
					return false
				}
			}
			return true
		}
		if s.inboundHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newInboundHealthCheck(healthFn, s.logger), *s.inboundHealthCheckConfig); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Started up without inbound health check! Double-check the YAML config," +
			" it needs at least the following path: healthCheck.listenAddress")
	}

	if s.outboundHealthCheckConfig != nil {
		healthFn := func() bool {
			// TODO: overly conservative right now. When there are multiple remotes, we should track their health separately
			for _, cc := range s.clusterConnections {
				if !cc.AcceptingOutboundTraffic() {
					return false
				}
			}
			return true
		}
		var err error
		if s.outboundHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newOutboundHealthCheck(healthFn, s.logger), *s.outboundHealthCheckConfig); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Started up without outbound health check! Double-check the YAML config," +
			" it needs at least the following path: outboundHealthCheck.listenAddress")
	}

	if s.metricsConfig != nil {
		if err := s.startMetricsHandler(s.lifetime, *s.metricsConfig); err != nil {
			return err
		}
	} else {
		s.logger.Warn(`Started up without metrics! Double-check the YAML config,` +
			` it needs at least the following path: metrics.prometheus.listenAddress`)
	}

	if s.shardManager != nil {
		if err := s.shardManager.Start(s.lifetime); err != nil {
			return err
		}
	}

	for _, v := range s.clusterConnections {
		v.Start()
	}

	s.logger.Info(fmt.Sprintf("Started Proxy with the following config:\n%s", s.Describe()))

	return nil
}

func (s *Proxy) Stop() {
	// All parts of the Proxy watch the "lifetime" context. Cancelling it will close all components
	// where necessary
	s.cancel()
}

func (s *Proxy) Done() <-chan struct{} {
	return s.lifetime.Done()
}

func (s *Proxy) Describe() string {
	sb := strings.Builder{}
	sb.WriteString("[proxy.Proxy with cluster connections:\n\t")
	for k, v := range s.clusterConnections {
		sb.WriteString(fmt.Sprintf("%s:", k.name))
		sb.WriteString(v.Describe())
		sb.WriteString("\n\t")
	}
	sb.WriteString("]")
	return sb.String()
}

// GetShardInfo returns debug information about shard distribution
func (s *Proxy) GetShardInfo() ShardDebugInfo {
	return s.shardManager.GetShardInfo()
}

// GetChannelInfo returns debug information about active channels
func (s *Proxy) GetChannelInfo() ChannelDebugInfo {
	remoteSendChannels := make(map[string]int)
	var totalSendChannels int

	// Collect remote send channel info first
	s.remoteSendChannelsMu.RLock()
	for shardID, ch := range s.remoteSendChannels {
		shardKey := ClusterShardIDtoString(shardID)
		remoteSendChannels[shardKey] = len(ch)
	}
	totalSendChannels = len(s.remoteSendChannels)
	s.remoteSendChannelsMu.RUnlock()

	localAckChannels := make(map[string]int)
	var totalAckChannels int

	// Collect local ack channel info separately
	s.localAckChannelsMu.RLock()
	for shardID, ch := range s.localAckChannels {
		shardKey := ClusterShardIDtoString(shardID)
		localAckChannels[shardKey] = len(ch)
	}
	totalAckChannels = len(s.localAckChannels)
	s.localAckChannelsMu.RUnlock()

	return ChannelDebugInfo{
		RemoteSendChannels: remoteSendChannels,
		LocalAckChannels:   localAckChannels,
		TotalSendChannels:  totalSendChannels,
		TotalAckChannels:   totalAckChannels,
	}
}

// SetRemoteSendChan registers a send channel for a specific shard ID
func (s *Proxy) SetRemoteSendChan(shardID history.ClusterShardID, sendChan chan RoutedMessage) {
	s.remoteSendChannelsMu.Lock()
	defer s.remoteSendChannelsMu.Unlock()
	s.remoteSendChannels[shardID] = sendChan
	s.logger.Info("Registered remote send channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
}

// GetRemoteSendChan retrieves the send channel for a specific shard ID
func (s *Proxy) GetRemoteSendChan(shardID history.ClusterShardID) (chan RoutedMessage, bool) {
	s.remoteSendChannelsMu.RLock()
	defer s.remoteSendChannelsMu.RUnlock()
	ch, exists := s.remoteSendChannels[shardID]
	return ch, exists
}

// GetAllRemoteSendChans returns a map of all remote send channels
func (s *Proxy) GetAllRemoteSendChans() map[history.ClusterShardID]chan RoutedMessage {
	s.remoteSendChannelsMu.RLock()
	defer s.remoteSendChannelsMu.RUnlock()

	// Create a copy of the map
	result := make(map[history.ClusterShardID]chan RoutedMessage, len(s.remoteSendChannels))
	for k, v := range s.remoteSendChannels {
		result[k] = v
	}
	return result
}

// GetRemoteSendChansByCluster returns a copy of remote send channels filtered by clusterID
func (s *Proxy) GetRemoteSendChansByCluster(clusterID int32) map[history.ClusterShardID]chan RoutedMessage {
	s.remoteSendChannelsMu.RLock()
	defer s.remoteSendChannelsMu.RUnlock()

	result := make(map[history.ClusterShardID]chan RoutedMessage)
	for k, v := range s.remoteSendChannels {
		if k.ClusterID == clusterID {
			result[k] = v
		}
	}
	return result
}

// RemoveRemoteSendChan removes the send channel for a specific shard ID
func (s *Proxy) RemoveRemoteSendChan(shardID history.ClusterShardID) {
	s.remoteSendChannelsMu.Lock()
	defer s.remoteSendChannelsMu.Unlock()
	delete(s.remoteSendChannels, shardID)
	s.logger.Info("Removed remote send channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
}

// SetLocalAckChan registers an ack channel for a specific shard ID
func (s *Proxy) SetLocalAckChan(shardID history.ClusterShardID, ackChan chan RoutedAck) {
	s.localAckChannelsMu.Lock()
	defer s.localAckChannelsMu.Unlock()
	s.localAckChannels[shardID] = ackChan
	s.logger.Info("Registered local ack channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
}

// GetLocalAckChan retrieves the ack channel for a specific shard ID
func (s *Proxy) GetLocalAckChan(shardID history.ClusterShardID) (chan RoutedAck, bool) {
	s.localAckChannelsMu.RLock()
	defer s.localAckChannelsMu.RUnlock()
	ch, exists := s.localAckChannels[shardID]
	return ch, exists
}

// RemoveLocalAckChan removes the ack channel for a specific shard ID
func (s *Proxy) RemoveLocalAckChan(shardID history.ClusterShardID) {
	s.localAckChannelsMu.Lock()
	defer s.localAckChannelsMu.Unlock()
	delete(s.localAckChannels, shardID)
	s.logger.Info("Removed local ack channel for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
}

// SetLocalReceiverCancelFunc registers a cancel function for a local receiver for a specific shard ID
func (s *Proxy) SetLocalReceiverCancelFunc(shardID history.ClusterShardID, cancelFunc context.CancelFunc) {
	s.localReceiverCancelFuncsMu.Lock()
	defer s.localReceiverCancelFuncsMu.Unlock()
	s.localReceiverCancelFuncs[shardID] = cancelFunc
	s.logger.Info("Registered local receiver cancel function for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
}

// GetLocalReceiverCancelFunc retrieves the cancel function for a local receiver for a specific shard ID
func (s *Proxy) GetLocalReceiverCancelFunc(shardID history.ClusterShardID) (context.CancelFunc, bool) {
	s.localReceiverCancelFuncsMu.RLock()
	defer s.localReceiverCancelFuncsMu.RUnlock()
	cancelFunc, exists := s.localReceiverCancelFuncs[shardID]
	return cancelFunc, exists
}

// RemoveLocalReceiverCancelFunc removes the cancel function for a local receiver for a specific shard ID
func (s *Proxy) RemoveLocalReceiverCancelFunc(shardID history.ClusterShardID) {
	s.localReceiverCancelFuncsMu.Lock()
	defer s.localReceiverCancelFuncsMu.Unlock()
	delete(s.localReceiverCancelFuncs, shardID)
	s.logger.Info("Removed local receiver cancel function for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(shardID)))
}

// TerminatePreviousLocalReceiver checks if there is a previous local receiver for this shard and terminates it if needed
func (s *Proxy) TerminatePreviousLocalReceiver(serverShardID history.ClusterShardID) {
	// Check if there's a previous cancel function for this shard
	if prevCancelFunc, exists := s.GetLocalReceiverCancelFunc(serverShardID); exists {
		s.logger.Info("Terminating previous local receiver for shard", tag.NewStringTag("shardID", ClusterShardIDtoString(serverShardID)))

		// Cancel the previous receiver's context
		prevCancelFunc()

		// Remove the cancel function from tracking
		s.RemoveLocalReceiverCancelFunc(serverShardID)

		// Also clean up the associated ack channel if it exists
		s.RemoveLocalAckChan(serverShardID)
	}
}
