package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/tests/testcore"

	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type simpleConfigProvider struct {
	cfg config.S2SProxyConfig
}

func (p *simpleConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	return p.cfg
}

type trackingUpstreamServer struct {
	address string
	conns   atomic.Int64
	count1  *atomic.Int64
	count2  *atomic.Int64
}

type trackingUpstream struct {
	servers []*trackingUpstreamServer
	mu      sync.RWMutex
}

func (u *trackingUpstream) selectLeastConn() *trackingUpstreamServer {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if len(u.servers) == 0 {
		return nil
	}

	selected := u.servers[0]
	minConns := selected.conns.Load()

	for i := 1; i < len(u.servers); i++ {
		conns := u.servers[i].conns.Load()
		if conns < minConns {
			minConns = conns
			selected = u.servers[i]
		}
	}

	if selected != nil {
		if selected == u.servers[0] {
			selected.count1.Add(1)
		} else if len(u.servers) > 1 && selected == u.servers[1] {
			selected.count2.Add(1)
		}
	}

	return selected
}

func (u *trackingUpstream) incrementConn(server *trackingUpstreamServer) {
	server.conns.Add(1)
}

func (u *trackingUpstream) decrementConn(server *trackingUpstreamServer) {
	server.conns.Add(-1)
}

type trackingTCPProxy struct {
	rules   []*trackingProxyRule
	logger  log.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	servers []net.Listener
}

type trackingProxyRule struct {
	ListenPort string
	Upstream   *trackingUpstream
}

func (p *trackingTCPProxy) Start() error {
	for _, rule := range p.rules {
		listener, err := net.Listen("tcp", ":"+rule.ListenPort)
		if err != nil {
			p.Stop()
			return fmt.Errorf("failed to listen on port %s: %w", rule.ListenPort, err)
		}
		p.servers = append(p.servers, listener)

		p.wg.Add(1)
		go p.handleListener(listener, rule)
	}

	return nil
}

func (p *trackingTCPProxy) Stop() {
	p.logger.Info("Stopping tracking TCP proxy")
	p.cancel()
	for _, server := range p.servers {
		p.logger.Info("Closing server", tag.NewStringTag("server", server.Addr().String()))
		_ = server.Close()
	}
	p.logger.Info("Waiting for goroutines to finish")
	p.wg.Wait()
	p.logger.Info("Tracking TCP proxy stopped")
}

func (p *trackingTCPProxy) handleListener(listener net.Listener, rule *trackingProxyRule) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		clientConn, err := listener.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				p.logger.Warn("failed to accept connection", tag.Error(err))
				continue
			}
		}

		p.wg.Add(1)
		go p.handleConnection(clientConn, rule)
	}
}

func (p *trackingTCPProxy) handleConnection(clientConn net.Conn, rule *trackingProxyRule) {
	defer p.wg.Done()
	defer func() { _ = clientConn.Close() }()

	select {
	case <-p.ctx.Done():
		return
	default:
	}

	upstream := rule.Upstream.selectLeastConn()
	if upstream == nil {
		p.logger.Error("no upstream servers available")
		return
	}

	rule.Upstream.incrementConn(upstream)
	defer rule.Upstream.decrementConn(upstream)

	serverConn, err := net.DialTimeout("tcp", upstream.address, 5*time.Second)
	if err != nil {
		p.logger.Warn("failed to connect to upstream", tag.NewStringTag("upstream", upstream.address), tag.Error(err))
		return
	}
	defer func() { _ = serverConn.Close() }()

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		<-p.ctx.Done()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(serverConn, clientConn)
		_ = serverConn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, serverConn)
		_ = clientConn.Close()
	}()

	wg.Wait()
}

func createLoadBalancer(
	logger log.Logger,
	listenPort string,
	upstreams []string,
	count1 *atomic.Int64,
	count2 *atomic.Int64,
) (*trackingTCPProxy, error) {
	trackingServers := make([]*trackingUpstreamServer, len(upstreams))
	for i, addr := range upstreams {
		trackingServers[i] = &trackingUpstreamServer{
			address: addr,
			count1:  count1,
			count2:  count2,
		}
	}

	trackingUpstream := &trackingUpstream{
		servers: trackingServers,
	}

	rules := []*trackingProxyRule{
		{
			ListenPort: listenPort,
			Upstream:   trackingUpstream,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	trackingProxy := &trackingTCPProxy{
		rules:  rules,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	err := trackingProxy.Start()
	if err != nil {
		return nil, err
	}

	return trackingProxy, nil
}

func createCluster(
	logger log.Logger,
	t testingT,
	clusterName string,
	numShards int,
	initialFailoverVersion int64,
	numHistoryHosts int,
) *testcore.TestCluster {
	clusterSuffix := common.GenerateRandomString(8)
	fullClusterName := fmt.Sprintf("%s-%s", clusterName, clusterSuffix)

	clusterConfig := &testcore.TestClusterConfig{
		ClusterMetadata: cluster.Config{
			EnableGlobalNamespace:    true,
			FailoverVersionIncrement: 10,
			MasterClusterName:        fullClusterName,
			CurrentClusterName:       fullClusterName,
			ClusterInformation: map[string]cluster.ClusterInformation{
				fullClusterName: {
					Enabled:                true,
					InitialFailoverVersion: initialFailoverVersion,
				},
			},
		},
		HistoryConfig: testcore.HistoryConfig{
			NumHistoryShards: int32(numShards),
			NumHistoryHosts:  numHistoryHosts,
		},
		DynamicConfigOverrides: map[dynamicconfig.Key]interface{}{
			dynamicconfig.NamespaceCacheRefreshInterval.Key(): time.Second,
			dynamicconfig.EnableReplicationStream.Key():       true,
			dynamicconfig.EnableReplicationTaskBatching.Key(): true,
		},
	}

	testClusterFactory := testcore.NewTestClusterFactory()
	logger = log.With(logger, tag.NewStringTag("clusterName", clusterName))

	testT := getTestingT(t)
	cluster, err := testClusterFactory.NewCluster(testT, clusterConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create cluster %s: %v", clusterName, err)
	}

	return cluster
}

func createProxy(
	logger log.Logger,
	t testingT,
	name string,
	inboundAddress string,
	outboundAddress string,
	muxAddress string,
	cluster *testcore.TestCluster,
	muxMode config.MuxMode,
	shardCountConfig config.ShardCountConfig,
	nodeName string,
	memberlistBindAddr string,
	memberlistBindPort int,
	memberlistJoinAddrs []string,
	proxyAddresses map[string]string,
) *s2sproxy.Proxy {
	var muxConnectionType config.ConnectionType
	var muxAddressInfo config.TCPTLSInfo
	if muxMode == config.ServerMode {
		muxConnectionType = config.ConnTypeMuxServer
		muxAddressInfo = config.TCPTLSInfo{
			ConnectionString: muxAddress,
		}
	} else {
		muxConnectionType = config.ConnTypeMuxClient
		muxAddressInfo = config.TCPTLSInfo{
			ConnectionString: muxAddress,
		}
	}

	cfg := &config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{
			{
				Name: name,
				LocalServer: config.ClusterDefinition{
					Connection: config.TransportInfo{
						ConnectionType: config.ConnTypeTCP,
						TcpClient: config.TCPTLSInfo{
							ConnectionString: cluster.Host().FrontendGRPCAddress(),
						},
						TcpServer: config.TCPTLSInfo{
							ConnectionString: outboundAddress,
						},
					},
				},
				RemoteServer: config.ClusterDefinition{
					Connection: config.TransportInfo{
						ConnectionType: muxConnectionType,
						MuxCount:       1,
						MuxAddressInfo: muxAddressInfo,
					},
				},
				ShardCountConfig: shardCountConfig,
			},
		},
	}

	if nodeName != "" && memberlistBindAddr != "" {
		cfg.ClusterConnections[0].MemberlistConfig = &config.MemberlistConfig{
			Enabled:        true,
			NodeName:       nodeName,
			BindAddr:       memberlistBindAddr,
			BindPort:       memberlistBindPort,
			JoinAddrs:      memberlistJoinAddrs,
			ProxyAddresses: proxyAddresses,
			TCPOnly:        true,
		}
	}

	configProvider := &simpleConfigProvider{cfg: *cfg}
	proxy := s2sproxy.NewProxy(configProvider, logger)
	if proxy == nil {
		t.Fatalf("Failed to create proxy %s", name)
	}

	err := proxy.Start()
	if err != nil {
		t.Fatalf("Failed to start proxy %s: %v", name, err)
	}

	logger.Info("Started proxy", tag.NewStringTag("name", name),
		tag.NewStringTag("inboundAddress", inboundAddress),
		tag.NewStringTag("outboundAddress", outboundAddress),
		tag.NewStringTag("muxAddress", muxAddress),
		tag.NewStringTag("muxMode", string(muxMode)),
		tag.NewStringTag("nodeName", nodeName),
	)

	return proxy
}

func configureRemoteCluster(
	logger log.Logger,
	t testingT,
	cluster *testcore.TestCluster,
	remoteClusterName string,
	proxyAddress string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := cluster.AdminClient().AddOrUpdateRemoteCluster(
		ctx,
		&adminservice.AddOrUpdateRemoteClusterRequest{
			FrontendAddress:               proxyAddress,
			EnableRemoteClusterConnection: true,
		},
	)
	if err != nil {
		t.Fatalf("Failed to configure remote cluster %s: %v", remoteClusterName, err)
	}
	logger.Info("Configured remote cluster",
		tag.NewStringTag("remoteClusterName", remoteClusterName),
		tag.NewStringTag("proxyAddress", proxyAddress),
		tag.NewStringTag("clusterName", cluster.ClusterName()),
	)
}

func removeRemoteCluster(
	logger log.Logger,
	t testingT,
	cluster *testcore.TestCluster,
	remoteClusterName string,
) {
	_, err := cluster.AdminClient().RemoveRemoteCluster(
		context.Background(),
		&adminservice.RemoveRemoteClusterRequest{
			ClusterName: remoteClusterName,
		},
	)
	if err != nil {
		t.Fatalf("Failed to remove remote cluster %s: %v", remoteClusterName, err)
	}
	logger.Info("Removed remote cluster",
		tag.NewStringTag("remoteClusterName", remoteClusterName),
		tag.NewStringTag("clusterName", cluster.ClusterName()),
	)
}

func waitForReplicationReady(
	logger log.Logger,
	t testingT,
	clusters ...*testcore.TestCluster,
) {
	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, cluster := range clusters {
		ready := false
		for i := 0; i < 25; i++ {
			_, err := cluster.HistoryClient().GetReplicationStatus(
				ctx,
				&historyservice.GetReplicationStatusRequest{},
			)
			if err == nil {
				ready = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !ready {
			t.Fatalf("Replication infrastructure not ready for cluster %s", cluster.ClusterName())
		}
	}

	time.Sleep(1 * time.Second)
}

type testingT interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

func getTestingT(t testingT) *testing.T {
	if testT, ok := t.(*testing.T); ok {
		return testT
	}
	if suiteT, ok := t.(interface{ T() *testing.T }); ok {
		return suiteT.T()
	}
	panic("testingT must be *testing.T or have T() method")
}

// GetFreePort returns an available TCP port by listening on localhost:0.
// This is useful for tests that need to allocate ports dynamically.
func GetFreePort() int {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(fmt.Sprintf("failed to get free port: %v", err))
	}
	defer func() {
		if err := l.Close(); err != nil {
			fmt.Printf("Failed to close listener: %v\n", err)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// GetLocalhostAddress returns a localhost address with a free port
func GetLocalhostAddress() string {
	return fmt.Sprintf("localhost:%d", GetFreePort())
}
