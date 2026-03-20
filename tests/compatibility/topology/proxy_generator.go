//go:build testcompatibility

package topology

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// startPort is the base port for per-proxy sequential allocation.
const startPort = 10001

// proxyYAMLConfig mirrors the YAML shape expected by s2s-proxy.
type proxyYAMLConfig struct {
	ClusterConnections []clusterConnYAML `yaml:"clusterConnections"`
	Profiling          profilingYAML     `yaml:"profiling"`
}

type profilingYAML struct {
	PprofAddress string `yaml:"pprofAddress"`
}

type clusterConnYAML struct {
	Name   string         `yaml:"name"`
	Local  clusterDefYAML `yaml:"local"`
	Remote clusterDefYAML `yaml:"remote"`
}

type clusterDefYAML struct {
	ConnectionType string      `yaml:"connectionType"`
	TcpClient      tcpInfoYAML `yaml:"tcpClient,omitempty"`
	TcpServer      tcpInfoYAML `yaml:"tcpServer,omitempty"`
	MuxCount       int         `yaml:"muxCount,omitempty"`
	MuxAddressInfo tcpInfoYAML `yaml:"muxAddressInfo,omitempty"`
}

type tcpInfoYAML struct {
	Address string `yaml:"address,omitempty"`
}

// proxySpec is the fully-resolved configuration for a proxy container.
type proxySpec struct {
	name          string
	networkAlias  string
	image         string
	network       Network
	configFile    ConfigFile
	exposedPorts  []string
	internalAddrs []string
}

// buildProxySpecs computes full-mesh proxy specs for N clusters.
//
// For N clusters there are N*(N-1)/2 cluster pairs. Each pair (i, j) where i < j
// assigns proxy-i as the mux-client and proxy-j as the mux-server. Ports are
// allocated sequentially per proxy starting from startPort.
func buildProxySpecs(clusters []ClusterHandle, imageTag string, net Network) []proxySpec {
	total := len(clusters)

	type pairConn struct {
		remoteName    string
		listenPort    int
		internalAddr  string
		connType      string
		muxAddr       string
		muxListenPort int // 0 for mux-client (not a server, not exposed)
		muxCount      int
	}

	// Collect connections for each proxy index.
	connsByProxy := make([][]pairConn, total)

	// Each proxy gets its own port counter; uniqueness within a container is sufficient.
	nextPort := make([]int, total)
	for i := range nextPort {
		nextPort[i] = startPort
	}

	for i := 0; i < total; i++ {
		for j := i + 1; j < total; j++ {
			aliasI := "proxy-" + clusters[i].Name()
			aliasJ := "proxy-" + clusters[j].Name()

			iPort := nextPort[i]
			nextPort[i]++
			jPort := nextPort[j]
			nextPort[j]++
			muxPort := nextPort[j]
			nextPort[j]++

			// proxy-i: mux-client for this pair
			connsByProxy[i] = append(connsByProxy[i], pairConn{
				remoteName:   clusters[j].Name(),
				listenPort:   iPort,
				internalAddr: containerAddr(aliasI, iPort),
				connType:     "mux-client",
				muxAddr:      containerAddr(aliasJ, muxPort),
				muxCount:     1,
			})

			// proxy-j: mux-server for this pair
			connsByProxy[j] = append(connsByProxy[j], pairConn{
				remoteName:    clusters[i].Name(),
				listenPort:    jPort,
				internalAddr:  containerAddr(aliasJ, jPort),
				connType:      "mux-server",
				muxAddr:       listenAddr(muxPort),
				muxListenPort: muxPort,
				muxCount:      1,
			})
		}
	}

	specs := make([]proxySpec, total)
	for i, cluster := range clusters {
		alias := "proxy-" + cluster.Name()
		conns := connsByProxy[i]

		// Build exposed ports and internal addrs.
		var exposedPorts []string
		internalAddrs := make([]string, len(conns))
		for k, conn := range conns {
			exposedPorts = append(exposedPorts, exposedPort(conn.listenPort))
			if conn.muxListenPort != 0 {
				exposedPorts = append(exposedPorts, exposedPort(conn.muxListenPort))
			}
			internalAddrs[k] = conn.internalAddr
		}

		// Render proxy YAML config inline.
		var clusterConns []clusterConnYAML
		for _, conn := range conns {
			clusterConns = append(clusterConns, clusterConnYAML{
				Name: conn.remoteName,
				Local: clusterDefYAML{
					ConnectionType: "tcp",
					TcpClient:      tcpInfoYAML{Address: cluster.InternalAddr()},
					TcpServer:      tcpInfoYAML{Address: listenAddr(conn.listenPort)},
				},
				Remote: clusterDefYAML{
					ConnectionType: conn.connType,
					MuxCount:       conn.muxCount,
					MuxAddressInfo: tcpInfoYAML{Address: conn.muxAddr},
				},
			})
		}
		cfg := proxyYAMLConfig{
			ClusterConnections: clusterConns,
			Profiling:          profilingYAML{PprofAddress: listenAddr(proxyPProfPort)},
		}
		data, err := yaml.Marshal(cfg)
		if err != nil {
			panic(fmt.Sprintf("marshal proxy config for %s: %v", alias, err))
		}

		specs[i] = proxySpec{
			name:          alias,
			networkAlias:  alias,
			image:         imageTag,
			network:       net,
			configFile:    ConfigFile{Content: string(data), MountPath: "/etc/proxy/config.yaml"},
			exposedPorts:  exposedPorts,
			internalAddrs: internalAddrs,
		}
	}

	return specs
}
