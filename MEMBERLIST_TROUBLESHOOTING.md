# Memberlist Network Troubleshooting

This guide helps resolve network connectivity issues with memberlist in the s2s-proxy.

## Common Issues

### UDP Ping Failures

**Symptoms:**
```
[DEBUG] memberlist: Failed UDP ping: proxy-node-a-2 (timeout reached)
[WARN] memberlist: Was able to connect to proxy-node-a-2 over TCP but UDP probes failed, network may be misconfigured
```

**Causes:**
- UDP traffic blocked by firewalls
- Running in containers without UDP port mapping
- Network security policies blocking UDP
- NAT/proxy configurations

**Solutions:**

#### 1. Use TCP-Only Mode (Recommended)

Update your configuration to use TCP-only transport:

```yaml
memberlist:
  enabled: true
  enableForwarding: true
  nodeName: "proxy-node-1"
  bindAddr: "0.0.0.0"
  bindPort: 7946
  joinAddrs:
    - "proxy-node-2:7946"
    - "proxy-node-3:7946"
  # TCP-only configuration
  tcpOnly: true           # Disable UDP entirely
  disableTCPPings: true   # Improve performance in TCP-only mode
  probeTimeoutMs: 1000    # Adjust for network latency
  probeIntervalMs: 2000   # Reduce probe frequency
```

#### 2. Open UDP Ports

If you want to keep UDP enabled:

**Docker/Kubernetes:**
```bash
# Expose UDP port in Docker
docker run -p 7946:7946/udp -p 7946:7946/tcp ...

# Kubernetes service
apiVersion: v1
kind: Service
spec:
  ports:
  - name: memberlist-tcp
    port: 7946
    protocol: TCP
  - name: memberlist-udp
    port: 7946
    protocol: UDP
```

**Firewall:**
```bash
# Linux iptables
iptables -A INPUT -p udp --dport 7946 -j ACCEPT
iptables -A INPUT -p tcp --dport 7946 -j ACCEPT

# AWS Security Groups - allow UDP/TCP 7946
```

#### 3. Adjust Bind Address

For container environments, use specific bind addresses:

```yaml
memberlist:
  bindAddr: "0.0.0.0"  # Listen on all interfaces
  # OR
  bindAddr: "10.0.0.1"  # Specific container IP
```

## Configuration Options

### Network Timing

```yaml
memberlist:
  probeTimeoutMs: 500    # Time to wait for ping response (default: 500ms)
  probeIntervalMs: 1000  # Time between health probes (default: 1s)
```

**Adjust based on network conditions:**
- **Fast networks**: Lower values (500ms timeout, 1s interval)
- **Slow/high-latency networks**: Higher values (1000ms timeout, 2s interval)
- **Unreliable networks**: Much higher values (2000ms timeout, 5s interval)

### Transport Modes

#### Local Network Mode (Default)
```yaml
memberlist:
  tcpOnly: false  # Uses both UDP and TCP
```
- Best for local networks
- Fastest failure detection
- Requires UDP connectivity

#### TCP-Only Mode
```yaml
memberlist:
  tcpOnly: true          # TCP transport only
  disableTCPPings: true  # Optimize for TCP-only
```
- Works in restricted networks
- Slightly slower failure detection
- More reliable in containerized environments

## Testing Connectivity

### 1. Test TCP Connectivity
```bash
# Test if TCP port is reachable
telnet proxy-node-2 7946
nc -zv proxy-node-2 7946
```

### 2. Test UDP Connectivity
```bash
# Test UDP port (if not using tcpOnly)
nc -u -zv proxy-node-2 7946
```

### 3. Monitor Memberlist Logs
Enable debug logging to see detailed memberlist behavior:
```bash
# Set log level to debug
export LOG_LEVEL=debug
./s2s-proxy start --config your-config.yaml
```

### 4. Check Debug Endpoint
Query the debug endpoint to see cluster status:
```bash
curl http://localhost:6060/debug/connections | jq .shard_info
```

## Example Configurations

### Docker Compose
```yaml
version: '3.8'
services:
  proxy1:
    image: s2s-proxy
    ports:
      - "7946:7946/tcp"
      - "7946:7946/udp"  # Only if not using tcpOnly
    environment:
      - CONFIG_PATH=/config/proxy.yaml
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: s2s-proxy
        ports:
        - containerPort: 7946
          protocol: TCP
        - containerPort: 7946
          protocol: UDP  # Only if not using tcpOnly
```

## Performance Impact

**UDP + TCP Mode:**
- Fastest failure detection (~1-2 seconds)
- Best for stable networks
- Requires UDP connectivity

**TCP-Only Mode:**
- Slightly slower failure detection (~2-5 seconds)
- More reliable in restricted environments
- Works everywhere TCP works

## Recommended Settings by Environment

### Local Development
```yaml
memberlist:
  tcpOnly: false
  probeTimeoutMs: 500
  probeIntervalMs: 1000
```

### Docker/Containers
```yaml
memberlist:
  tcpOnly: true
  disableTCPPings: true
  probeTimeoutMs: 1000
  probeIntervalMs: 2000
```

### Kubernetes
```yaml
memberlist:
  tcpOnly: true
  disableTCPPings: true
  probeTimeoutMs: 1500
  probeIntervalMs: 3000
```

### High-Latency/Unreliable Networks
```yaml
memberlist:
  tcpOnly: true
  disableTCPPings: true
  probeTimeoutMs: 2000
  probeIntervalMs: 5000
```
