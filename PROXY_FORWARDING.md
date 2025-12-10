# Proxy-to-Proxy Forwarding

This document describes the proxy-to-proxy forwarding functionality that enables distributed shard management across multiple s2s-proxy instances.

## Overview

The proxy-to-proxy forwarding mechanism allows multiple proxy instances to work together as a cluster, where each proxy instance owns a subset of shards. When a replication stream request comes to a proxy that doesn't own the target shard, it automatically forwards the request to the proxy instance that does own that shard.

## Architecture

```
Client → Proxy A (Inbound) → Proxy B (Inbound) → Target Server
        (Forward)              (Owner)
```

## How It Works

1. **Shard Ownership**: Using consistent hashing via HashiCorp memberlist, each proxy instance is assigned ownership of specific shards
2. **Ownership Check**: When a `StreamWorkflowReplicationMessages` request arrives on an **inbound connection** with **forwarding enabled**, the proxy checks if it owns the required shard
3. **Forwarding**: If another proxy owns the shard, the request is forwarded to that proxy (only for inbound connections with forwarding enabled)
4. **Bidirectional Streaming**: The forwarding proxy acts as a transparent relay, forwarding both requests and responses

## Key Components

### Shard Manager
- **Interface**: `ShardManager` with methods for shard ownership and proxy address resolution
- **Implementation**: Uses memberlist for cluster membership and consistent hashing for shard distribution
- **Methods**:
  - `IsLocalShard(shardID)` - Check if this proxy owns a shard
  - `GetShardOwner(shardID)` - Get the node name that owns a shard
  - `GetProxyAddress(nodeName)` - Get the service address for a proxy node

### Forwarding Logic
- **Location**: `StreamWorkflowReplicationMessages` in `adminservice.go`
- **Conditions**: Forwards only when:
  - **Inbound connection** (`s.IsInbound == true`)
  - **Memberlist enabled** (`memberlist.enabled == true`)
  - **Forwarding enabled** (`memberlist.enableForwarding == true`)
- **Checks**: Two shard ownership checks (only for inbound):
  1. `clientShardID` - the incoming shard from the client
  2. `serverShardID` - the target shard (after LCM remapping if applicable)
- **Forwarding Function**: `forwardToProxy()` handles the bidirectional streaming

### Configuration

```yaml
memberlist:
  enabled: true
  # Enable proxy-to-proxy forwarding
  enableForwarding: true
  nodeName: "proxy-node-1"
  bindAddr: "0.0.0.0"
  bindPort: 7946
  joinAddrs:
    - "proxy-node-2:7946"
    - "proxy-node-3:7946"
  shardStrategy: "consistent"
  proxyAddresses:
    "proxy-node-1": "localhost:7001"
    "proxy-node-2": "proxy-node-2:7001"
    "proxy-node-3": "proxy-node-3:7001"
```

## Metrics

The following Prometheus metrics track forwarding operations:

- `shard_distribution` - Number of shards handled by each proxy instance
- `shard_forwarding_total` - Total forwarding operations (labels: from_node, to_node, result)
- `memberlist_cluster_size` - Number of nodes in the memberlist cluster
- `memberlist_events_total` - Memberlist events (join/leave)

## Benefits

1. **Horizontal Scaling**: Add more proxy instances to handle more shards
2. **High Availability**: Automatic shard redistribution when proxies fail
3. **Load Distribution**: Shards are evenly distributed across proxy instances
4. **Transparent**: Clients don't need to know about shard ownership
5. **Configurable**: Can enable cluster coordination without forwarding via `enableForwarding: false`
6. **Backward Compatible**: Works with existing setups when memberlist is disabled

## Limitations

- Forwarding adds one additional network hop for non-local shards
- Requires careful configuration of proxy addresses for inter-proxy communication
- Uses insecure gRPC connections for proxy-to-proxy communication (can be enhanced with TLS)

## Example Deployment

For a 3-proxy cluster handling temporal replication:

1. **proxy-node-1**: Handles shards 0, 3, 6, 9, ...
2. **proxy-node-2**: Handles shards 1, 4, 7, 10, ...  
3. **proxy-node-3**: Handles shards 2, 5, 8, 11, ...

When a replication stream for shard 7 comes to proxy-node-1, it will automatically forward to proxy-node-2. 