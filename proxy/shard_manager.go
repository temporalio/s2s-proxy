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
		// RegisterShard registers a clientShardID as owned by this proxy instance
		RegisterShard(clientShardID history.ClusterShardID)
		// UnregisterShard removes a clientShardID from this proxy's ownership
		UnregisterShard(clientShardID history.ClusterShardID)
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
		DeliverAckToShardOwner(srcShard history.ClusterShardID, routedAck *RoutedAck, proxy *Proxy, shutdownChan channel.ShutdownOnce, logger log.Logger, ack int64, allowForward bool) bool
		// DeliverMessagesToShardOwner routes replication messages to the appropriate shard owner (local or remote)
		DeliverMessagesToShardOwner(targetShard history.ClusterShardID, routedMsg *RoutedMessage, proxy *Proxy, shutdownChan channel.ShutdownOnce, logger log.Logger) bool
		// SetOnPeerJoin registers a callback invoked when a new peer joins
		SetOnPeerJoin(handler func(nodeName string))
		// SetOnPeerLeave registers a callback invoked when a peer leaves.
		SetOnPeerLeave(handler func(nodeName string))
		// New: notify when local shard set changes
		SetOnLocalShardChange(handler func(shard history.ClusterShardID, added bool))
		// New: notify when remote shard set changes for a peer
		SetOnRemoteShardChange(handler func(peer string, shard history.ClusterShardID, added bool))

		SetIntraProxyManager(intraMgr *intraProxyManager)
		GetIntraProxyManager() *intraProxyManager
	}

	shardManagerImpl struct {
		config      *config.MemberlistConfig
		logger      log.Logger
		ml          *memberlist.Memberlist
		delegate    *shardDelegate
		mutex       sync.RWMutex
		localAddr   string
		started     bool
		onPeerJoin  func(nodeName string)
		onPeerLeave func(nodeName string)
		// New callbacks
		onLocalShardChange  func(shard history.ClusterShardID, added bool)
		onRemoteShardChange func(peer string, shard history.ClusterShardID, added bool)
		// Local shards owned by this node, keyed by short id
		localShards map[string]ShardInfo
		intraMgr    *intraProxyManager
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
func NewShardManager(configProvider config.ConfigProvider, logger log.Logger) (ShardManager, error) {
	cfg := configProvider.GetS2SProxyConfig().MemberlistConfig
	if cfg == nil || !cfg.Enabled {
		return &noopShardManager{}, nil
	}

	delegate := &shardDelegate{
		logger: logger,
	}

	sm := &shardManagerImpl{
		config:      cfg,
		logger:      logger,
		delegate:    delegate,
		localShards: make(map[string]ShardInfo),
		intraMgr:    nil,
	}

	delegate.manager = sm

	return sm, nil
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

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.started {
		sm.logger.Info("Shard manager already started")
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
	sm.addLocalShard(clientShardID)
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
}

func (sm *shardManagerImpl) UnregisterShard(clientShardID history.ClusterShardID) {
	sm.removeLocalShard(clientShardID)
	sm.mutex.Lock()
	delete(sm.localShards, ClusterShardIDtoShortString(clientShardID))
	sm.mutex.Unlock()
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
	if sm.config.ProxyAddresses == nil {
		return "", false
	}
	addr, found := sm.config.ProxyAddresses[nodeName]
	return addr, found
}

func (sm *shardManagerImpl) GetNodeName() string {
	return sm.config.NodeName
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
		NodeName:          sm.config.NodeName,
		LocalShards:       localShardMap,
		LocalShardCount:   len(localShardMap),
		RemoteShards:      remoteShardsMap,
		RemoteShardCounts: remoteShardCounts,
	}
}

func (sm *shardManagerImpl) GetShardOwner(shard history.ClusterShardID) (string, bool) {
	// FIXME: improve this: store remote shards in a map in the shardManagerImpl
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
	proxy *Proxy,
	shutdownChan channel.ShutdownOnce,
	logger log.Logger,
	ack int64,
	allowForward bool,
) bool {
	logger = log.With(logger, tag.NewStringTag("sourceShard", ClusterShardIDtoString(sourceShard)), tag.NewInt64("ack", ack))
	if ackCh, ok := proxy.GetLocalAckChan(sourceShard); ok {
		select {
		case ackCh <- *routedAck:
			logger.Info("Delivered ACK to local shard owner")
			return true
		case <-shutdownChan.Channel():
			return false
		}
	}
	if !allowForward {
		logger.Warn("No local ack channel for source shard, forwarding ACK to shard owner is not allowed")
		return false
	}

	// Attempt remote delivery via intra-proxy when enabled and shard is remote
	if owner, ok := sm.GetShardOwner(sourceShard); ok && owner != sm.config.NodeName {
		if addr, found := sm.GetProxyAddress(owner); found {
			clientShard := routedAck.TargetShard
			serverShard := sourceShard
			mgr := proxy.GetIntraProxyManager(migrationId{owner})
			// Synchronous send to preserve ordering
			if err := mgr.sendAck(context.Background(), owner, clientShard, serverShard, proxy, routedAck.Req); err != nil {
				logger.Error("Failed to forward ACK to shard owner via intra-proxy", tag.Error(err), tag.NewStringTag("owner", owner), tag.NewStringTag("addr", addr))
				return false
			}
			logger.Info("Forwarded ACK to shard owner via intra-proxy", tag.NewStringTag("owner", owner), tag.NewStringTag("addr", addr))
			return true
		}
		logger.Warn("Owner proxy address not found for shard")
		return false
	}

	logger.Warn("No remote shard owner found for source shard")
	return false
}

// DeliverMessagesToShardOwner routes replication messages to the local target shard owner
// or forwards to the remote owner via intra-proxy stream synchronously.
func (sm *shardManagerImpl) DeliverMessagesToShardOwner(
	targetShard history.ClusterShardID,
	routedMsg *RoutedMessage,
	proxy *Proxy,
	shutdownChan channel.ShutdownOnce,
	logger log.Logger,
) bool {
	// Try local delivery first
	if ch, ok := proxy.GetRemoteSendChan(targetShard); ok {
		select {
		case ch <- *routedMsg:
			return true
		case <-shutdownChan.Channel():
			return false
		}
	}

	// Attempt remote delivery via intra-proxy when enabled and shard is remote
	if sm.config != nil {
		if owner, ok := sm.GetShardOwner(targetShard); ok && owner != sm.config.NodeName {
			if addr, found := sm.GetProxyAddress(owner); found {
				if mgr := sm.GetIntraProxyManager(); mgr != nil {
					resp := routedMsg.Resp
					if err := mgr.sendReplicationMessages(context.Background(), owner, targetShard, routedMsg.SourceShard, proxy, resp); err != nil {
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
}

func (sm *shardManagerImpl) GetIntraProxyManager() *intraProxyManager {
	return sm.intraMgr
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

// shardDelegate implements memberlist.Delegate
func (sd *shardDelegate) NodeMeta(limit int) []byte {
	state := NodeShardState{
		NodeName: sd.manager.config.NodeName,
		Shards:   sd.manager.localShards,
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

	// Inform listeners about remote shard changes
	if sd.manager != nil && sd.manager.onRemoteShardChange != nil {
		added := msg.Type == "register"

		// if shard is previously registered as local shard, but now is registered as remote shard,
		// check if the remote shard is newer than the local shard. If so, unregister the local shard.
		if added {
			localShard, ok := sd.manager.localShards[ClusterShardIDtoShortString(msg.ClientShard)]
			if ok {
				if localShard.Created.Before(msg.Timestamp) {
					sd.manager.UnregisterShard(msg.ClientShard)
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

func (sm *shardManagerImpl) addLocalShard(shard history.ClusterShardID) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	key := ClusterShardIDtoShortString(shard)
	sm.localShards[key] = ShardInfo{ID: shard, Created: time.Now()}

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
func (nsm *noopShardManager) GetNodeName() string                                 { return "" }
func (nsm *noopShardManager) GetMemberNodes() []string                            { return []string{} }
func (nsm *noopShardManager) GetLocalShards() map[string]history.ClusterShardID {
	return make(map[string]history.ClusterShardID)
}
func (nsm *noopShardManager) GetRemoteShardsForPeer(string) (map[string]NodeShardState, error) {
	return make(map[string]NodeShardState), nil
}
func (nsm *noopShardManager) GetShardInfo() ShardDebugInfo {
	return ShardDebugInfo{
		Enabled:           false,
		NodeName:          "",
		LocalShards:       make(map[string]history.ClusterShardID),
		LocalShardCount:   0,
		ClusterNodes:      []string{},
		ClusterSize:       0,
		RemoteShards:      make(map[string]string),
		RemoteShardCounts: make(map[string]int),
	}
}

func (nsm *noopShardManager) SetOnPeerJoin(handler func(nodeName string))  {}
func (nsm *noopShardManager) SetOnPeerLeave(handler func(nodeName string)) {}
func (nsm *noopShardManager) SetOnLocalShardChange(handler func(shard history.ClusterShardID, added bool)) {
}
func (nsm *noopShardManager) SetOnRemoteShardChange(handler func(peer string, shard history.ClusterShardID, added bool)) {
}

func (nsm *noopShardManager) DeliverAckToShardOwner(srcShard history.ClusterShardID, routedAck *RoutedAck, proxy *Proxy, shutdownChan channel.ShutdownOnce, logger log.Logger, ack int64, allowForward bool) bool {
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

func (nsm *noopShardManager) DeliverMessagesToShardOwner(targetShard history.ClusterShardID, routedMsg *RoutedMessage, proxy *Proxy, shutdownChan channel.ShutdownOnce, logger log.Logger) bool {
	if proxy != nil {
		if ch, ok := proxy.GetRemoteSendChan(targetShard); ok {
			select {
			case ch <- *routedMsg:
				return true
			case <-shutdownChan.Channel():
				return false
			}
		}
	}
	return false
}

func (nsm *noopShardManager) SetIntraProxyManager(intraMgr *intraProxyManager) {
}
func (nsm *noopShardManager) GetIntraProxyManager() *intraProxyManager {
	return nil
}
