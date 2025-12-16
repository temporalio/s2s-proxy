package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// ShardManager manages distributed shard ownership across proxy instances
	ShardManager interface {
		// Start initializes the memberlist cluster and starts the manager
		Start(lifetime context.Context) error
		// Stop shuts down the manager and leaves the cluster
		Stop()
		// RegisterShard registers a clientShardID as owned by this proxy instance and returns the registration timestamp
		RegisterShard(clientShardID history.ClusterShardID) time.Time
		// UnregisterShard removes a clientShardID from this proxy's ownership only if the timestamp matches
		UnregisterShard(clientShardID history.ClusterShardID, expectedRegisteredAt time.Time)
		// GetProxyAddress returns the proxy service address for the given node name
		GetProxyAddress(nodeName string) (string, bool)
		// IsLocalShard checks if this proxy instance owns the given shard
		IsLocalShard(clientShardID history.ClusterShardID) bool
		// GetNodeName returns the name of this proxy instance
		GetNodeName() string
		// GetMemberNodes returns all active proxy nodes in the cluster
		GetMemberNodes() []string
		// GetLocalShards returns all shards currently handled by this proxy instance, keyed by short id
		GetLocalShards() map[string]history.ClusterShardID
		// GetRemoteShardsForPeer returns all shards owned by the specified peer node, keyed by short id
		GetRemoteShardsForPeer(peerNodeName string) (map[string]NodeShardState, error)
		// GetShardInfo returns debug information about shard distribution
		GetShardInfo() ShardDebugInfo
		// GetShardOwner returns the node name that owns the given shard
		GetShardOwner(shard history.ClusterShardID) (string, bool)
		// DeliverAckToShardOwner routes an ACK request to the appropriate shard owner (local or remote)
		DeliverAckToShardOwner(srcShard history.ClusterShardID, routedAck *RoutedAck, clusterConnection *ClusterConnection, shutdownChan channel.ShutdownOnce, logger log.Logger, ack int64, allowForward bool) bool
		// DeliverMessagesToShardOwner routes replication messages to the appropriate shard owner (local or remote)
		DeliverMessagesToShardOwner(targetShard history.ClusterShardID, routedMsg *RoutedMessage, clusterConnection *ClusterConnection, shutdownChan channel.ShutdownOnce, logger log.Logger) bool
		// SetOnPeerJoin registers a callback invoked when a new peer joins
		SetOnPeerJoin(handler func(nodeName string))
		// SetOnPeerLeave registers a callback invoked when a peer leaves.
		SetOnPeerLeave(handler func(nodeName string))
		// New: notify when local shard set changes
		SetOnLocalShardChange(handler func(shard history.ClusterShardID, added bool))
		// New: notify when remote shard set changes for a peer
		SetOnRemoteShardChange(handler func(peer string, shard history.ClusterShardID, added bool))
	}

	shardManagerImpl struct {
		memberlistConfig *config.MemberlistConfig
		logger           log.Logger
		ml               *memberlist.Memberlist
		delegate         *shardDelegate
		mutex            sync.RWMutex
		localAddr        string
		started          bool
		onPeerJoin       func(nodeName string)
		onPeerLeave      func(nodeName string)
		// New callbacks
		onLocalShardChange  func(shard history.ClusterShardID, added bool)
		onRemoteShardChange func(peer string, shard history.ClusterShardID, added bool)
		// Local shards owned by this node, keyed by short id
		localShards map[string]ShardInfo
		intraMgr    *intraProxyManager
		// Join retry control
		stopJoinRetry   chan struct{}
		joinWg          sync.WaitGroup
		joinLoopRunning bool
	}

	// shardDelegate implements memberlist.Delegate for shard state management
	shardDelegate struct {
		manager *shardManagerImpl
		logger  log.Logger
	}

	// ShardInfo describes a local shard and its creation time
	ShardInfo struct {
		ID      history.ClusterShardID `json:"id"`
		Created time.Time              `json:"created"`
	}

	// ShardMessage represents shard ownership changes broadcast to cluster
	ShardMessage struct {
		Type        string                 `json:"type"` // "register" or "unregister"
		NodeName    string                 `json:"node"`
		ClientShard history.ClusterShardID `json:"shard"`
		Timestamp   time.Time              `json:"timestamp"`
	}

	// NodeShardState represents all shards owned by a node
	NodeShardState struct {
		NodeName string               `json:"node"`
		Shards   map[string]ShardInfo `json:"shards"`
		Updated  time.Time            `json:"updated"`
	}
)

// NewShardManager creates a new shard manager instance
func NewShardManager(memberlistConfig *config.MemberlistConfig, logger log.Logger) ShardManager {
	delegate := &shardDelegate{
		logger: logger,
	}

	sm := &shardManagerImpl{
		memberlistConfig: memberlistConfig,
		logger:           logger,
		delegate:         delegate,
		localShards:      make(map[string]ShardInfo),
		intraMgr:         nil,
		stopJoinRetry:    make(chan struct{}),
	}

	delegate.manager = sm

	return sm
}

// SetOnPeerJoin registers a callback invoked on new peer joins.
func (sm *shardManagerImpl) SetOnPeerJoin(handler func(nodeName string)) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.onPeerJoin = handler
}

// SetOnPeerLeave registers a callback invoked when a peer leaves.
func (sm *shardManagerImpl) SetOnPeerLeave(handler func(nodeName string)) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.onPeerLeave = handler
}

// SetOnLocalShardChange registers local shard change callback.
func (sm *shardManagerImpl) SetOnLocalShardChange(handler func(shard history.ClusterShardID, added bool)) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.onLocalShardChange = handler
}

// SetOnRemoteShardChange registers remote shard change callback.
func (sm *shardManagerImpl) SetOnRemoteShardChange(handler func(peer string, shard history.ClusterShardID, added bool)) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.onRemoteShardChange = handler
}

func (sm *shardManagerImpl) Start(lifetime context.Context) error {
	sm.logger.Info("Starting shard manager")
	if sm.memberlistConfig == nil {
		sm.logger.Info("Shard manager not configured, skipping")
		return nil
	}

	if sm.started {
		sm.logger.Info("Shard manager already started")
		return nil
	}

	// Configure memberlist
	var mlConfig *memberlist.Config
	if sm.memberlistConfig.TCPOnly {
		// Use LAN config as base for TCP-only mode
		mlConfig = memberlist.DefaultLANConfig()
		mlConfig.DisableTcpPings = sm.memberlistConfig.DisableTCPPings
		// Set default timeouts for TCP-only if not specified
		if sm.memberlistConfig.ProbeTimeoutMs == 0 {
			mlConfig.ProbeTimeout = 1 * time.Second
		}
		if sm.memberlistConfig.ProbeIntervalMs == 0 {
			mlConfig.ProbeInterval = 2 * time.Second
		}
	} else {
		mlConfig = memberlist.DefaultLocalConfig()
	}
	mlConfig.Name = sm.memberlistConfig.NodeName
	mlConfig.BindAddr = sm.memberlistConfig.BindAddr
	mlConfig.BindPort = sm.memberlistConfig.BindPort
	mlConfig.AdvertiseAddr = sm.memberlistConfig.BindAddr
	mlConfig.AdvertisePort = sm.memberlistConfig.BindPort

	mlConfig.Delegate = sm.delegate
	mlConfig.Events = &shardEventDelegate{manager: sm, logger: sm.logger}

	// Configure custom timeouts if specified
	if sm.memberlistConfig.ProbeTimeoutMs > 0 {
		mlConfig.ProbeTimeout = time.Duration(sm.memberlistConfig.ProbeTimeoutMs) * time.Millisecond
	}
	if sm.memberlistConfig.ProbeIntervalMs > 0 {
		mlConfig.ProbeInterval = time.Duration(sm.memberlistConfig.ProbeIntervalMs) * time.Millisecond
	}

	sm.logger.Info("Creating memberlist",
		tag.NewStringTag("nodeName", mlConfig.Name),
		tag.NewStringTag("bindAddr", mlConfig.BindAddr),
		tag.NewStringTag("bindPort", fmt.Sprintf("%d", mlConfig.BindPort)),
		tag.NewBoolTag("tcpOnly", sm.memberlistConfig.TCPOnly),
		tag.NewBoolTag("disableTcpPings", mlConfig.DisableTcpPings),
		tag.NewStringTag("probeTimeout", mlConfig.ProbeTimeout.String()),
		tag.NewStringTag("probeInterval", mlConfig.ProbeInterval.String()))

	// Create memberlist with timeout protection
	type result struct {
		ml  *memberlist.Memberlist
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		ml, err := memberlist.Create(mlConfig)
		resultCh <- result{ml: ml, err: err}
	}()

	var ml *memberlist.Memberlist
	select {
	case res := <-resultCh:
		ml = res.ml
		if res.err != nil {
			return fmt.Errorf("failed to create memberlist: %w", res.err)
		}
		sm.logger.Info("Memberlist created successfully")
	case <-time.After(10 * time.Second):
		return fmt.Errorf("memberlist.Create() timed out after 10s - check bind address/port availability")
	}

	sm.mutex.Lock()
	sm.ml = ml
	sm.localAddr = fmt.Sprintf("%s:%d", sm.memberlistConfig.BindAddr, sm.memberlistConfig.BindPort)
	sm.started = true

	sm.logger.Info("Shard manager base initialization complete",
		tag.NewStringTag("node", sm.memberlistConfig.NodeName),
		tag.NewStringTag("addr", sm.localAddr))

	sm.mutex.Unlock()

	// Join existing cluster if configured
	if len(sm.memberlistConfig.JoinAddrs) > 0 {
		sm.startJoinLoop()
	}

	sm.logger.Info("Shard manager started",
		tag.NewStringTag("node", sm.memberlistConfig.NodeName),
		tag.NewStringTag("addr", sm.localAddr))

	context.AfterFunc(lifetime, func() {
		sm.Stop()
	})
	return nil
}

func (sm *shardManagerImpl) Stop() {
	sm.mutex.Lock()

	if !sm.started || sm.ml == nil {
		sm.mutex.Unlock()
		return
	}
	sm.mutex.Unlock()

	// Stop any ongoing join retry
	close(sm.stopJoinRetry)
	sm.joinWg.Wait()

	// Leave the cluster gracefully
	err := sm.ml.Leave(5 * time.Second)
	if err != nil {
		sm.logger.Error("Error leaving memberlist cluster", tag.Error(err))
	}

	err = sm.ml.Shutdown()
	if err != nil {
		sm.logger.Error("Error shutting down memberlist", tag.Error(err))
	}

	sm.mutex.Lock()
	sm.started = false
	sm.mutex.Unlock()
	sm.logger.Info("Shard manager stopped")
}

// startJoinLoop starts the join retry loop if not already running
func (sm *shardManagerImpl) startJoinLoop() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.joinLoopRunning {
		sm.logger.Info("Join loop already running, skipping")
		return
	}

	sm.logger.Info("Starting join loop")
	sm.joinLoopRunning = true
	sm.joinWg.Add(1)
	go sm.retryJoinCluster()
}

// retryJoinCluster attempts to join the cluster infinitely with exponential backoff
func (sm *shardManagerImpl) retryJoinCluster() {
	defer func() {
		sm.joinWg.Done()
		sm.mutex.Lock()
		sm.joinLoopRunning = false
		sm.mutex.Unlock()
	}()

	const (
		initialInterval = 2 * time.Second
		maxInterval     = 60 * time.Second
	)

	interval := initialInterval
	attempt := 0

	sm.logger.Info("Starting join retry loop",
		tag.NewStringTag("joinAddrs", fmt.Sprintf("%v", sm.memberlistConfig.JoinAddrs)))

	for {
		attempt++

		sm.mutex.RLock()
		ml := sm.ml
		joinAddrs := sm.memberlistConfig.JoinAddrs
		sm.mutex.RUnlock()

		if ml == nil {
			sm.logger.Warn("Memberlist not initialized, stopping retry")
			return
		}

		sm.logger.Info("Attempting to join cluster",
			tag.NewStringTag("attempt", strconv.Itoa(attempt)),
			tag.NewStringTag("joinAddrs", fmt.Sprintf("%v", joinAddrs)))

		num, err := ml.Join(joinAddrs)
		if err != nil {
			sm.logger.Warn("Failed to join cluster", tag.Error(err))

			// Exponential backoff with cap
			select {
			case <-sm.stopJoinRetry:
				sm.logger.Info("Join retry cancelled")
				return
			case <-time.After(interval):
				interval *= 2
				if interval > maxInterval {
					interval = maxInterval
				}
			}
		} else {
			sm.logger.Info("Successfully joined memberlist cluster",
				tag.NewStringTag("members", strconv.Itoa(num)),
				tag.NewStringTag("attempt", strconv.Itoa(attempt)))
			return
		}
	}
}

func (sm *shardManagerImpl) RegisterShard(clientShardID history.ClusterShardID) time.Time {
	sm.logger.Info("RegisterShard", tag.NewStringTag("shard", ClusterShardIDtoString(clientShardID)))
	registeredAt := sm.addLocalShard(clientShardID)
	sm.broadcastShardChange("register", clientShardID)

	// Trigger memberlist metadata update to propagate NodeMeta to other nodes
	if sm.ml != nil {
		if err := sm.ml.UpdateNode(0); err != nil { // 0 timeout means immediate update
			sm.logger.Warn("Failed to update memberlist node metadata", tag.Error(err))
		}
	}
	// Notify listeners
	if sm.onLocalShardChange != nil {
		sm.onLocalShardChange(clientShardID, true)
	}
	return registeredAt
}

func (sm *shardManagerImpl) UnregisterShard(clientShardID history.ClusterShardID, expectedRegisteredAt time.Time) {
	sm.logger.Info("UnregisterShard", tag.NewStringTag("shard", ClusterShardIDtoString(clientShardID)))

	// Only unregister if the registration timestamp matches (prevents old senders from removing new registrations)
	sm.mutex.Lock()
	key := ClusterShardIDtoShortString(clientShardID)
	if shardInfo, exists := sm.localShards[key]; exists && shardInfo.Created.Equal(expectedRegisteredAt) {
		delete(sm.localShards, key)
		// Update metrics after local shards change
		sm.mutex.Unlock()

		sm.removeLocalShard(clientShardID)
		sm.broadcastShardChange("unregister", clientShardID)

		// Trigger memberlist metadata update to propagate NodeMeta to other nodes
		if sm.ml != nil {
			if err := sm.ml.UpdateNode(0); err != nil { // 0 timeout means immediate update
				sm.logger.Warn("Failed to update memberlist node metadata", tag.Error(err))
			}
		}
		// Notify listeners
		if sm.onLocalShardChange != nil {
			sm.onLocalShardChange(clientShardID, false)
		}
		sm.logger.Info("UnregisterShard completed", tag.NewStringTag("shard", ClusterShardIDtoString(clientShardID)))
	} else {
		sm.mutex.Unlock()
		sm.logger.Info("Skipped unregistering shard (timestamp mismatch or already unregistered)", tag.NewStringTag("shard", ClusterShardIDtoString(clientShardID)))
	}
}

func (sm *shardManagerImpl) IsLocalShard(clientShardID history.ClusterShardID) bool {
	if !sm.started {
		return true // If not using memberlist, handle locally
	}

	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	_, found := sm.localShards[ClusterShardIDtoShortString(clientShardID)]
	return found
}

func (sm *shardManagerImpl) GetProxyAddress(nodeName string) (string, bool) {
	// TODO: get the proxy address from the memberlist metadata
	if sm.memberlistConfig == nil || sm.memberlistConfig.ProxyAddresses == nil {
		return "", false
	}
	addr, found := sm.memberlistConfig.ProxyAddresses[nodeName]
	return addr, found
}

func (sm *shardManagerImpl) GetNodeName() string {
	if sm.memberlistConfig == nil {
		return ""
	}
	return sm.memberlistConfig.NodeName
}

func (sm *shardManagerImpl) GetMemberNodes() []string {
	if !sm.started || sm.ml == nil {
		return []string{sm.memberlistConfig.NodeName}
	}

	// Use a timeout to prevent deadlocks when memberlist is busy
	membersChan := make(chan []*memberlist.Node, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				sm.logger.Error("Panic in GetMemberNodes", tag.NewStringTag("error", fmt.Sprintf("%v", r)))
			}
		}()
		membersChan <- sm.ml.Members()
	}()

	select {
	case members := <-membersChan:
		nodes := make([]string, len(members))
		for i, member := range members {
			nodes[i] = member.Name
		}
		return nodes
	case <-time.After(100 * time.Millisecond):
		// Timeout: return cached node name to prevent hanging
		sm.logger.Warn("GetMemberNodes timeout, returning self node",
			tag.NewStringTag("node", sm.memberlistConfig.NodeName))
		return []string{sm.memberlistConfig.NodeName}
	}
}

func (sm *shardManagerImpl) GetLocalShards() map[string]history.ClusterShardID {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	shards := make(map[string]history.ClusterShardID, len(sm.localShards))
	for k, v := range sm.localShards {
		shards[k] = v.ID
	}
	return shards
}

func (sm *shardManagerImpl) GetShardInfo() ShardDebugInfo {
	localShardMap := sm.GetLocalShards()
	remoteShards, err := sm.GetRemoteShardsForPeer("")
	if err != nil {
		sm.logger.Error("Failed to get remote shards", tag.Error(err))
	}

	remoteShardsMap := make(map[string]string)
	remoteShardCounts := make(map[string]int)

	for nodeName, shards := range remoteShards {
		for _, shard := range shards.Shards {
			shardKey := ClusterShardIDtoShortString(shard.ID)
			remoteShardsMap[shardKey] = nodeName
		}
		remoteShardCounts[nodeName] = len(shards.Shards)
	}

	return ShardDebugInfo{
		Enabled:           true,
		NodeName:          sm.GetNodeName(),
		LocalShards:       localShardMap,
		LocalShardCount:   len(localShardMap),
		RemoteShards:      remoteShardsMap,
		RemoteShardCounts: remoteShardCounts,
	}
}

func (sm *shardManagerImpl) GetShardOwner(shard history.ClusterShardID) (string, bool) {
	remoteShards, err := sm.GetRemoteShardsForPeer("")
	if err != nil {
		sm.logger.Error("Failed to get remote shards", tag.Error(err))
	}
	for nodeName, shards := range remoteShards {
		for _, s := range shards.Shards {
			if s.ID == shard {
				return nodeName, true
			}
		}
	}
	return "", false
}

// GetRemoteShardsForPeer returns all shards owned by the specified peer node.
// Non-blocking: uses memberlist metadata and tolerates timeouts by returning a best-effort set.
func (sm *shardManagerImpl) GetRemoteShardsForPeer(peerNodeName string) (map[string]NodeShardState, error) {
	result := make(map[string]NodeShardState)
	if sm.ml == nil {
		return result, nil
	}

	// Read members with a short timeout to avoid blocking debug paths
	membersChan := make(chan []*memberlist.Node, 1)
	go func() {
		defer func() { _ = recover() }()
		sm.mutex.RLock()
		defer sm.mutex.RUnlock()
		membersChan <- sm.ml.Members()
	}()

	var members []*memberlist.Node
	select {
	case members = <-membersChan:
	case <-time.After(100 * time.Millisecond):
		sm.logger.Warn("GetRemoteShardsForPeer timeout")
		return result, fmt.Errorf("timeout")
	}

	for _, member := range members {
		if member == nil || len(member.Meta) == 0 {
			continue
		}
		if member.Name == sm.GetNodeName() {
			continue
		}
		if peerNodeName != "" && member.Name != peerNodeName {
			continue
		}
		var nodeState NodeShardState
		if err := json.Unmarshal(member.Meta, &nodeState); err != nil {
			continue
		}
		result[member.Name] = nodeState
	}

	return result, nil
}

// DeliverAckToShardOwner routes an ACK to the local shard owner or records intent for remote forwarding.
func (sm *shardManagerImpl) DeliverAckToShardOwner(
	sourceShard history.ClusterShardID,
	routedAck *RoutedAck,
	clusterConnection *ClusterConnection,
	shutdownChan channel.ShutdownOnce,
	logger log.Logger,
	ack int64,
	allowForward bool,
) bool {
	logger = log.With(logger, tag.NewStringTag("sourceShard", ClusterShardIDtoString(sourceShard)), tag.NewInt64("ack", ack))
	if ackCh, ok := clusterConnection.localAckChannels[sourceShard]; ok {
		delivered := false
		func() {
			defer func() {
				if panicErr := recover(); panicErr != nil {
					logger.Warn("Failed to deliver ACK to local shard owner (channel closed)")
				}
			}()
			select {
			case ackCh <- *routedAck:
				logger.Info("Delivered ACK to local shard owner")
				delivered = true
			case <-shutdownChan.Channel():
				// Shutdown signal received
			}
		}()
		if delivered {
			return true
		}
		if shutdownChan.IsShutdown() {
			return false
		}
	}
	if !allowForward {
		logger.Warn("No local ack channel for source shard, forwarding ACK to shard owner is not allowed")
		return false
	}

	// Attempt remote delivery via intra-proxy when enabled and shard is remote
	if sm.memberlistConfig != nil {
		if owner, ok := sm.GetShardOwner(sourceShard); ok && owner != sm.memberlistConfig.NodeName {
			if addr, found := sm.GetProxyAddress(owner); found {
				clientShard := routedAck.TargetShard
				serverShard := sourceShard
				mgr := clusterConnection.intraMgr
				// Synchronous send to preserve ordering
				if err := mgr.sendAck(context.Background(), owner, clientShard, serverShard, routedAck.Req); err != nil {
					logger.Error("Failed to forward ACK to shard owner via intra-proxy", tag.Error(err), tag.NewStringTag("owner", owner), tag.NewStringTag("addr", addr))
					return false
				}
				logger.Info("Forwarded ACK to shard owner via intra-proxy", tag.NewStringTag("owner", owner), tag.NewStringTag("addr", addr))
				return true
			}
			logger.Warn("Owner proxy address not found for shard")
			return false
		}
	}

	logger.Warn("No remote shard owner found for source shard")
	return false
}

// DeliverMessagesToShardOwner routes replication messages to the local target shard owner
// or forwards to the remote owner via intra-proxy stream synchronously.
func (sm *shardManagerImpl) DeliverMessagesToShardOwner(
	targetShard history.ClusterShardID,
	routedMsg *RoutedMessage,
	clusterConnection *ClusterConnection,
	shutdownChan channel.ShutdownOnce,
	logger log.Logger,
) bool {
	logger = log.With(logger, tag.NewStringTag("task-target-shard", ClusterShardIDtoString(targetShard)))

	// Try local delivery first
	if ch, ok := clusterConnection.remoteSendChannels[targetShard]; ok {
		delivered := false
		func() {
			defer func() {
				if panicErr := recover(); panicErr != nil {
					logger.Warn("Failed to deliver messages to local shard owner (channel closed)")
				}
			}()
			select {
			case ch <- *routedMsg:
				logger.Info("Delivered messages to local shard owner")
				delivered = true
			case <-shutdownChan.Channel():
				// Shutdown signal received
			}
		}()
		if delivered {
			return true
		}
		if shutdownChan.IsShutdown() {
			return false
		}
	}

	// Attempt remote delivery via intra-proxy when enabled and shard is remote
	if sm.memberlistConfig != nil {
		if owner, ok := sm.GetShardOwner(targetShard); ok && owner != sm.memberlistConfig.NodeName {
			if addr, found := sm.GetProxyAddress(owner); found {
				if mgr := sm.GetIntraProxyManager(); mgr != nil {
					resp := routedMsg.Resp
					if err := mgr.sendReplicationMessages(context.Background(), owner, targetShard, routedMsg.SourceShard, resp); err != nil {
						logger.Error("Failed to forward replication messages to shard owner via intra-proxy", tag.Error(err), tag.NewStringTag("owner", owner), tag.NewStringTag("addr", addr))
						return false
					}
					return true
				}
			} else {
				logger.Warn("Owner proxy address not found for target shard", tag.NewStringTag("owner", owner), tag.NewStringTag("shard", ClusterShardIDtoString(targetShard)))
			}
		}
	}

	logger.Warn("No local send channel for target shard", tag.NewStringTag("targetShard", ClusterShardIDtoString(targetShard)))
	return false
}

func (sm *shardManagerImpl) SetIntraProxyManager(intraMgr *intraProxyManager) {
	sm.intraMgr = intraMgr

	// Wire memberlist peer-join callback to reconcile intra-proxy receivers for local/remote pairs
	sm.SetOnPeerJoin(func(nodeName string) {
		sm.logger.Info("OnPeerJoin", tag.NewStringTag("nodeName", nodeName))
		defer sm.logger.Info("OnPeerJoin done", tag.NewStringTag("nodeName", nodeName))
		sm.intraMgr.Notify()
		// proxy.intraMgr.ReconcilePeerStreams(proxy, nodeName)
	})

	// Wire peer-leave to cleanup intra-proxy resources for that peer
	sm.SetOnPeerLeave(func(nodeName string) {
		sm.logger.Info("OnPeerLeave", tag.NewStringTag("nodeName", nodeName))
		defer sm.logger.Info("OnPeerLeave done", tag.NewStringTag("nodeName", nodeName))
		sm.intraMgr.Notify()
		// proxy.intraMgr.ReconcilePeerStreams(proxy, nodeName)
	})

	// Wire local shard changes to reconcile intra-proxy receivers
	sm.SetOnLocalShardChange(func(shard history.ClusterShardID, added bool) {
		sm.logger.Info("OnLocalShardChange", tag.NewStringTag("shard", ClusterShardIDtoString(shard)), tag.NewStringTag("added", strconv.FormatBool(added)))
		defer sm.logger.Info("OnLocalShardChange done", tag.NewStringTag("shard", ClusterShardIDtoString(shard)), tag.NewStringTag("added", strconv.FormatBool(added)))
		sm.intraMgr.Notify()
		// proxy.intraMgr.ReconcilePeerStreams(proxy, "")
	})

	// Wire remote shard changes to reconcile intra-proxy receivers
	sm.SetOnRemoteShardChange(func(peer string, shard history.ClusterShardID, added bool) {
		sm.logger.Info("OnRemoteShardChange", tag.NewStringTag("peer", peer), tag.NewStringTag("shard", ClusterShardIDtoString(shard)), tag.NewStringTag("added", strconv.FormatBool(added)))
		defer sm.logger.Info("OnRemoteShardChange done", tag.NewStringTag("peer", peer), tag.NewStringTag("shard", ClusterShardIDtoString(shard)), tag.NewStringTag("added", strconv.FormatBool(added)))
		sm.intraMgr.Notify()
		// proxy.intraMgr.ReconcilePeerStreams(proxy, peer)
	})
}

func (sm *shardManagerImpl) GetIntraProxyManager() *intraProxyManager {
	return sm.intraMgr
}

func (sm *shardManagerImpl) broadcastShardChange(msgType string, shard history.ClusterShardID) {
	if !sm.started || sm.ml == nil || sm.memberlistConfig == nil {
		return
	}

	msg := ShardMessage{
		Type:        msgType,
		NodeName:    sm.memberlistConfig.NodeName,
		ClientShard: shard,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		sm.logger.Error("Failed to marshal shard message", tag.Error(err))
		return
	}

	for _, member := range sm.ml.Members() {
		// Skip sending to self node
		if member.Name == sm.memberlistConfig.NodeName {
			continue
		}

		// Send in goroutine to make it non-blocking
		go func(m *memberlist.Node) {
			err := sm.ml.SendReliable(m, data)
			if err != nil {
				sm.logger.Error("Failed to broadcast shard change",
					tag.Error(err),
					tag.NewStringTag("target_node", m.Name))
			}
		}(member)
	}
}

// shardDelegate implements memberlist.Delegate
func (sd *shardDelegate) NodeMeta(limit int) []byte {
	if sd.manager == nil || sd.manager.memberlistConfig == nil {
		return nil
	}
	// Copy shard map under read lock to avoid concurrent map iteration/modification
	sd.manager.mutex.RLock()
	shardsCopy := make(map[string]ShardInfo, len(sd.manager.localShards))
	for k, v := range sd.manager.localShards {
		shardsCopy[k] = v
	}
	nodeName := sd.manager.memberlistConfig.NodeName
	sd.manager.mutex.RUnlock()

	state := NodeShardState{
		NodeName: nodeName,
		Shards:   shardsCopy,
		Updated:  time.Now(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		sd.logger.Error("Failed to marshal node meta", tag.Error(err))
		return nil
	}

	if len(data) > limit {
		// If metadata is too large, just send node name
		return []byte(sd.manager.memberlistConfig.NodeName)
	}

	return data
}

func (sd *shardDelegate) NotifyMsg(data []byte) {
	var msg ShardMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		sd.logger.Error("Failed to unmarshal shard message", tag.Error(err))
		return
	}

	sd.logger.Info("Received shard message",
		tag.NewStringTag("type", msg.Type),
		tag.NewStringTag("node", msg.NodeName),
		tag.NewStringTag("shard", ClusterShardIDtoString(msg.ClientShard)))

	// Inform listeners about remote shard changes
	if sd.manager != nil && sd.manager.onRemoteShardChange != nil {
		added := msg.Type == "register"

		// if shard is previously registered as local shard, but now is registered as remote shard,
		// check if the remote shard is newer than the local shard. If so, unregister the local shard.
		if added {
			localShard, ok := sd.manager.localShards[ClusterShardIDtoShortString(msg.ClientShard)]
			if ok {
				if localShard.Created.Before(msg.Timestamp) {
					// Force unregister the local shard by passing its own timestamp
					sd.manager.UnregisterShard(msg.ClientShard, localShard.Created)
				}
			}
		}

		sd.manager.onRemoteShardChange(msg.NodeName, msg.ClientShard, added)
	}
}

func (sd *shardDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	// Not implementing broadcasts for now
	return nil
}

func (sd *shardDelegate) LocalState(join bool) []byte {
	return sd.NodeMeta(4096) // TODO: set this to a reasonable value
}

func (sd *shardDelegate) MergeRemoteState(buf []byte, join bool) {
	var state NodeShardState
	if err := json.Unmarshal(buf, &state); err != nil {
		sd.logger.Error("Failed to unmarshal remote state", tag.Error(err))
		return
	}

	sd.logger.Info("Merged remote shard state",
		tag.NewStringTag("node", state.NodeName),
		tag.NewStringTag("shards", strconv.Itoa(len(state.Shards))),
		tag.NewStringTag("state", fmt.Sprintf("%+v", state)))
}

func (sm *shardManagerImpl) addLocalShard(shard history.ClusterShardID) time.Time {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	key := ClusterShardIDtoShortString(shard)
	now := time.Now()
	sm.localShards[key] = ShardInfo{ID: shard, Created: now}

	return now
}

func (sm *shardManagerImpl) removeLocalShard(shard history.ClusterShardID) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	key := ClusterShardIDtoShortString(shard)
	delete(sm.localShards, key)
}

// shardEventDelegate handles memberlist cluster events
type shardEventDelegate struct {
	manager *shardManagerImpl
	logger  log.Logger
}

func (sed *shardEventDelegate) NotifyJoin(node *memberlist.Node) {
	sed.logger.Info("Node joined cluster",
		tag.NewStringTag("node", node.Name),
		tag.NewStringTag("addr", node.Addr.String()))
}

func (sed *shardEventDelegate) NotifyLeave(node *memberlist.Node) {
	sed.logger.Info("Node left cluster",
		tag.NewStringTag("node", node.Name),
		tag.NewStringTag("addr", node.Addr.String()))

	// If we're now isolated and have join addresses configured, restart join loop
	if sed.manager != nil && sed.manager.ml != nil && sed.manager.memberlistConfig != nil {
		numMembers := sed.manager.ml.NumMembers()
		if numMembers == 1 && len(sed.manager.memberlistConfig.JoinAddrs) > 0 {
			sed.logger.Info("Node is now isolated, restarting join loop",
				tag.NewStringTag("numMembers", strconv.Itoa(numMembers)))
			sed.manager.startJoinLoop()
		}
	}
}

func (sed *shardEventDelegate) NotifyUpdate(node *memberlist.Node) {
	sed.logger.Info("Node updated",
		tag.NewStringTag("node", node.Name),
		tag.NewStringTag("addr", node.Addr.String()))
}
