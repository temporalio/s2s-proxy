package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
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
		// RegisterShard registers a clientShardID as owned by this proxy instance
		RegisterShard(clientShardID history.ClusterShardID)
		// UnregisterShard removes a clientShardID from this proxy's ownership
		UnregisterShard(clientShardID history.ClusterShardID)
		// GetShardOwner returns the proxy node name that owns the given shard
		GetShardOwner(clientShardID history.ClusterShardID) (string, bool)
		// GetProxyAddress returns the proxy service address for the given node name
		GetProxyAddress(nodeName string) (string, bool)
		// IsLocalShard checks if this proxy instance owns the given shard
		IsLocalShard(clientShardID history.ClusterShardID) bool
		// GetMemberNodes returns all active proxy nodes in the cluster
		GetMemberNodes() []string
		// GetLocalShards returns all shards currently handled by this proxy instance
		GetLocalShards() []history.ClusterShardID
		// GetShardInfo returns debug information about shard distribution
		GetShardInfo() ShardDebugInfo
		// DeliverAckToShardOwner routes an ACK request to the appropriate shard owner (local or remote)
		DeliverAckToShardOwner(srcShard history.ClusterShardID, routedAck *RoutedAck, proxy *Proxy, shutdownChan channel.ShutdownOnce, logger log.Logger) bool
	}

	shardManagerImpl struct {
		config    *config.MemberlistConfig
		logger    log.Logger
		ml        *memberlist.Memberlist
		delegate  *shardDelegate
		mutex     sync.RWMutex
		localAddr string
		started   bool
	}

	// shardDelegate implements memberlist.Delegate for shard state management
	shardDelegate struct {
		manager     *shardManagerImpl
		logger      log.Logger
		localShards map[string]history.ClusterShardID // key: "clusterID:shardID"
		mutex       sync.RWMutex
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
		NodeName string                            `json:"node"`
		Shards   map[string]history.ClusterShardID `json:"shards"`
		Updated  time.Time                         `json:"updated"`
	}
)

// NewShardManager creates a new shard manager instance
func NewShardManager(configProvider config.ConfigProvider, logger log.Logger) (ShardManager, error) {
	cfg := configProvider.GetS2SProxyConfig().MemberlistConfig
	if cfg == nil || !cfg.Enabled {
		return &noopShardManager{}, nil
	}

	delegate := &shardDelegate{
		logger:      logger,
		localShards: make(map[string]history.ClusterShardID),
	}

	sm := &shardManagerImpl{
		config:   cfg,
		logger:   logger,
		delegate: delegate,
	}

	delegate.manager = sm

	return sm, nil
}

func (sm *shardManagerImpl) Start(lifetime context.Context) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.started {
		return nil
	}

	// Configure memberlist
	var mlConfig *memberlist.Config
	if sm.config.TCPOnly {
		mlConfig = memberlist.DefaultWANConfig()
		// Disable UDP for restricted networks
		mlConfig.DisableTcpPings = sm.config.DisableTCPPings
	} else {
		mlConfig = memberlist.DefaultLocalConfig()
	}

	mlConfig.Name = sm.config.NodeName
	mlConfig.BindAddr = sm.config.BindAddr
	mlConfig.BindPort = sm.config.BindPort
	mlConfig.Delegate = sm.delegate
	mlConfig.Events = &shardEventDelegate{manager: sm, logger: sm.logger}

	// Configure timeouts if specified
	if sm.config.ProbeTimeoutMs > 0 {
		mlConfig.ProbeTimeout = time.Duration(sm.config.ProbeTimeoutMs) * time.Millisecond
	}
	if sm.config.ProbeIntervalMs > 0 {
		mlConfig.ProbeInterval = time.Duration(sm.config.ProbeIntervalMs) * time.Millisecond
	}

	// Create memberlist
	ml, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}

	sm.ml = ml
	sm.localAddr = fmt.Sprintf("%s:%d", sm.config.BindAddr, sm.config.BindPort)

	// Join existing cluster if configured
	if len(sm.config.JoinAddrs) > 0 {
		num, err := ml.Join(sm.config.JoinAddrs)
		if err != nil {
			sm.logger.Warn("Failed to join some cluster members", tag.Error(err))
		}
		sm.logger.Info("Joined memberlist cluster", tag.NewStringTag("members", strconv.Itoa(num)))
	}

	sm.started = true
	sm.logger.Info("Shard manager started",
		tag.NewStringTag("node", sm.config.NodeName),
		tag.NewStringTag("addr", sm.localAddr))

	context.AfterFunc(lifetime, func() {
		sm.Stop()
	})
	return nil
}

func (sm *shardManagerImpl) Stop() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if !sm.started || sm.ml == nil {
		return
	}

	// Leave the cluster gracefully
	err := sm.ml.Leave(5 * time.Second)
	if err != nil {
		sm.logger.Error("Error leaving memberlist cluster", tag.Error(err))
	}

	err = sm.ml.Shutdown()
	if err != nil {
		sm.logger.Error("Error shutting down memberlist", tag.Error(err))
	}

	sm.started = false
	sm.logger.Info("Shard manager stopped")
}

func (sm *shardManagerImpl) RegisterShard(clientShardID history.ClusterShardID) {
	sm.delegate.addLocalShard(clientShardID)
	sm.broadcastShardChange("register", clientShardID)

	// Trigger memberlist metadata update to propagate NodeMeta to other nodes
	if sm.ml != nil {
		if err := sm.ml.UpdateNode(0); err != nil { // 0 timeout means immediate update
			sm.logger.Warn("Failed to update memberlist node metadata", tag.Error(err))
		}
	}
}

func (sm *shardManagerImpl) UnregisterShard(clientShardID history.ClusterShardID) {
	sm.delegate.removeLocalShard(clientShardID)
	sm.broadcastShardChange("unregister", clientShardID)

	// Trigger memberlist metadata update to propagate NodeMeta to other nodes
	if sm.ml != nil {
		if err := sm.ml.UpdateNode(0); err != nil { // 0 timeout means immediate update
			sm.logger.Warn("Failed to update memberlist node metadata", tag.Error(err))
		}
	}
}

func (sm *shardManagerImpl) GetShardOwner(clientShardID history.ClusterShardID) (string, bool) {
	if !sm.started {
		return "", false
	}

	// Use consistent hashing to determine shard owner
	return sm.consistentHashOwner(clientShardID), true
}

func (sm *shardManagerImpl) IsLocalShard(clientShardID history.ClusterShardID) bool {
	if !sm.started {
		return true // If not using memberlist, handle locally
	}

	owner, found := sm.GetShardOwner(clientShardID)
	return found && owner == sm.config.NodeName
}

func (sm *shardManagerImpl) GetProxyAddress(nodeName string) (string, bool) {
	if sm.config.ProxyAddresses == nil {
		return "", false
	}
	addr, found := sm.config.ProxyAddresses[nodeName]
	return addr, found
}

func (sm *shardManagerImpl) GetMemberNodes() []string {
	if !sm.started || sm.ml == nil {
		return []string{sm.config.NodeName}
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
			tag.NewStringTag("node", sm.config.NodeName))
		return []string{sm.config.NodeName}
	}
}

func (sm *shardManagerImpl) GetLocalShards() []history.ClusterShardID {
	sm.delegate.mutex.RLock()
	defer sm.delegate.mutex.RUnlock()

	shards := make([]history.ClusterShardID, 0, len(sm.delegate.localShards))
	for _, shard := range sm.delegate.localShards {
		shards = append(shards, shard)
	}
	return shards
}

func (sm *shardManagerImpl) GetShardInfo() ShardDebugInfo {
	localShards := sm.GetLocalShards()
	clusterNodes := sm.GetMemberNodes()

	// Build remote shard maps by querying memberlist metadata directly
	remoteShards := make(map[string]string)
	remoteShardCounts := make(map[string]int)

	// Initialize counts for all nodes
	for _, node := range clusterNodes {
		remoteShardCounts[node] = 0
	}

	// Count local shards for this node
	remoteShardCounts[sm.config.NodeName] = len(localShards)

	// Collect shard ownership information from all cluster members
	if sm.ml != nil {
		for _, member := range sm.ml.Members() {
			if len(member.Meta) > 0 {
				var nodeState NodeShardState
				if err := json.Unmarshal(member.Meta, &nodeState); err == nil {
					nodeName := nodeState.NodeName
					if nodeName != "" {
						remoteShardCounts[nodeName] = len(nodeState.Shards)

						// Add remote shards (exclude local node)
						if nodeName != sm.config.NodeName {
							for _, shard := range nodeState.Shards {
								shardKey := fmt.Sprintf("%d:%d", shard.ClusterID, shard.ShardID)
								remoteShards[shardKey] = nodeName
							}
						}
					}
				}
			}
		}
	}

	return ShardDebugInfo{
		Enabled:           true,
		ForwardingEnabled: sm.config.EnableForwarding,
		NodeName:          sm.config.NodeName,
		LocalShards:       localShards,
		LocalShardCount:   len(localShards),
		ClusterNodes:      clusterNodes,
		ClusterSize:       len(clusterNodes),
		RemoteShards:      remoteShards,
		RemoteShardCounts: remoteShardCounts,
	}
}

// DeliverAckToShardOwner routes an ACK to the local shard owner or records intent for remote forwarding.
func (sm *shardManagerImpl) DeliverAckToShardOwner(
	sourceShard history.ClusterShardID,
	routedAck *RoutedAck,
	proxy *Proxy,
	shutdownChan channel.ShutdownOnce,
	logger log.Logger,
) bool {
	if ackCh, ok := proxy.GetLocalAckChan(sourceShard); ok {
		select {
		case ackCh <- *routedAck:
			return true
		case <-shutdownChan.Channel():
			return false
		}
	} else {
		logger.Warn("No local ack channel for source shard", tag.NewStringTag("shard", ClusterShardIDtoString(sourceShard)))
	}
	return false
}

func (sm *shardManagerImpl) broadcastShardChange(msgType string, shard history.ClusterShardID) {
	if !sm.started || sm.ml == nil {
		return
	}

	msg := ShardMessage{
		Type:        msgType,
		NodeName:    sm.config.NodeName,
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
		if member.Name == sm.config.NodeName {
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

func (sm *shardManagerImpl) consistentHashOwner(shard history.ClusterShardID) string {
	nodes := sm.GetMemberNodes()
	if len(nodes) == 0 {
		return sm.config.NodeName
	}

	// Sort nodes for consistent ordering
	sort.Strings(nodes)

	// Hash the shard ID
	h := fnv.New32a()
	shardKey := fmt.Sprintf("%d:%d", shard.ClusterID, shard.ShardID)
	h.Write([]byte(shardKey))
	hash := h.Sum32()

	// Use consistent hashing to determine owner
	return nodes[hash%uint32(len(nodes))]
}

// shardDelegate implements memberlist.Delegate
func (sd *shardDelegate) NodeMeta(limit int) []byte {
	sd.mutex.RLock()
	defer sd.mutex.RUnlock()

	state := NodeShardState{
		NodeName: sd.manager.config.NodeName,
		Shards:   sd.localShards,
		Updated:  time.Now(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		sd.logger.Error("Failed to marshal node meta", tag.Error(err))
		return nil
	}

	if len(data) > limit {
		// If metadata is too large, just send node name
		return []byte(sd.manager.config.NodeName)
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
}

func (sd *shardDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	// Not implementing broadcasts for now
	return nil
}

func (sd *shardDelegate) LocalState(join bool) []byte {
	return sd.NodeMeta(512)
}

func (sd *shardDelegate) MergeRemoteState(buf []byte, join bool) {
	var state NodeShardState
	if err := json.Unmarshal(buf, &state); err != nil {
		sd.logger.Error("Failed to unmarshal remote state", tag.Error(err))
		return
	}

	sd.logger.Info("Merged remote shard state",
		tag.NewStringTag("node", state.NodeName),
		tag.NewStringTag("shards", strconv.Itoa(len(state.Shards))))
}

func (sd *shardDelegate) addLocalShard(shard history.ClusterShardID) {
	sd.mutex.Lock()
	defer sd.mutex.Unlock()

	key := fmt.Sprintf("%d:%d", shard.ClusterID, shard.ShardID)
	sd.localShards[key] = shard
}

func (sd *shardDelegate) removeLocalShard(shard history.ClusterShardID) {
	sd.mutex.Lock()
	defer sd.mutex.Unlock()

	key := fmt.Sprintf("%d:%d", shard.ClusterID, shard.ShardID)
	delete(sd.localShards, key)
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
}

func (sed *shardEventDelegate) NotifyUpdate(node *memberlist.Node) {
	sed.logger.Info("Node updated",
		tag.NewStringTag("node", node.Name),
		tag.NewStringTag("addr", node.Addr.String()))
}

// noopShardManager provides a no-op implementation when memberlist is disabled
type noopShardManager struct{}

func (nsm *noopShardManager) Start(_ context.Context) error                       { return nil }
func (nsm *noopShardManager) Stop()                                               {}
func (nsm *noopShardManager) RegisterShard(history.ClusterShardID)                {}
func (nsm *noopShardManager) UnregisterShard(history.ClusterShardID)              {}
func (nsm *noopShardManager) GetShardOwner(history.ClusterShardID) (string, bool) { return "", false }
func (nsm *noopShardManager) GetProxyAddress(string) (string, bool)               { return "", false }
func (nsm *noopShardManager) IsLocalShard(history.ClusterShardID) bool            { return true }
func (nsm *noopShardManager) GetMemberNodes() []string                            { return []string{} }
func (nsm *noopShardManager) GetLocalShards() []history.ClusterShardID {
	return []history.ClusterShardID{}
}
func (nsm *noopShardManager) GetShardInfo() ShardDebugInfo {
	return ShardDebugInfo{
		Enabled:           false,
		ForwardingEnabled: false,
		NodeName:          "",
		LocalShards:       []history.ClusterShardID{},
		LocalShardCount:   0,
		ClusterNodes:      []string{},
		ClusterSize:       0,
		RemoteShards:      make(map[string]string),
		RemoteShardCounts: make(map[string]int),
	}
}

func (nsm *noopShardManager) DeliverAckToShardOwner(srcShard history.ClusterShardID, routedAck *RoutedAck, proxy *Proxy, shutdownChan channel.ShutdownOnce, logger log.Logger) bool {
	if proxy != nil {
		if ackCh, ok := proxy.GetLocalAckChan(srcShard); ok {
			select {
			case ackCh <- *routedAck:
				return true
			case <-shutdownChan.Channel():
				return false
			}
		}
	}
	return false
}
