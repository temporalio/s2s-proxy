package proxy

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/endtoendtest/testservices"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

func init() {
	_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "error")
	mux.MuxManagerStartDelay = 0
}

func getDynamicPorts(t *testing.T, num int) []string {
	listeners := make([]net.Listener, num)
	output := make([]string, num)
	for i := range num {
		var err error
		listeners[i], err = net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		output[i] = listeners[i].Addr().String()
	}
	// Ensure the listeners aren't sitting on the ports
	for _, l := range listeners {
		_ = l.Close()
	}
	t.Log("Prepared ports:", output)
	return output
}

type pairedLocalClusterConnection struct {
	localTemporal        *testservices.TemporalServerWithListen
	cancelLocalTemporal  context.CancelFunc
	remoteTemporal       *testservices.TemporalServerWithListen
	cancelRemoteTemporal context.CancelFunc
	localCC              *ClusterConnection
	cancelLocalCC        context.CancelFunc
	remoteCC             *ClusterConnection
	cancelRemoteCC       context.CancelFunc
	clientFromLocal      *grpc.ClientConn
	clientFromRemote     *grpc.ClientConn
	addresses            plccAddresses
}
type plccAddresses struct {
	localTemporalAddr   string
	remoteTemporalAddr  string
	localProxyOutbound  string
	remoteProxyInbound  string
	localProxyInbound   string
	remoteProxyOutbound string
}

func getDynamicPlccAddresses(t *testing.T) plccAddresses {
	a := plccAddresses{}
	addresses := getDynamicPorts(t, 6)
	// Server listening ports can be visualized like this:
	//  0          1                          2       3                             4        5
	// local, ->outbound | local proxy | <-inbound, inbound-> | remote proxy | <-outbound, remote
	// Outbound traffic goes from 0->1->3->5
	// Inbound traffic goes from 5->4->2->0
	a.localTemporalAddr, a.remoteTemporalAddr = addresses[0], addresses[5]
	a.localProxyOutbound, a.remoteProxyInbound = addresses[1], addresses[3]
	a.localProxyInbound, a.remoteProxyOutbound = addresses[2], addresses[4]
	return a
}

func (plcc *pairedLocalClusterConnection) StartAll(t *testing.T) {
	plcc.localTemporal.Start()
	var localCtx context.Context
	localCtx, plcc.cancelLocalTemporal = context.WithCancel(t.Context())
	context.AfterFunc(localCtx, plcc.localTemporal.Stop)
	plcc.remoteTemporal.Start()
	var remoteCtx context.Context
	remoteCtx, plcc.cancelRemoteTemporal = context.WithCancel(t.Context())
	context.AfterFunc(remoteCtx, plcc.remoteTemporal.Stop)
	plcc.remoteCC.Start()
	plcc.localCC.Start()
}

func makeTCPClusterConfig(name string, localServer string, localToRemoteServer string, remoteToLocalServer string, remoteServer string) config.ClusterConnConfig {
	return config.ClusterConnConfig{
		Name: name,
		LocalServer: config.ClusterDefinition{
			Connection: config.TransportInfo{
				ConnectionType: config.ConnTypeTCP,
				TcpServer: config.TCPTLSInfo{
					ConnectionString: remoteToLocalServer,
				},
				TcpClient: config.TCPTLSInfo{
					ConnectionString: localServer,
				},
			},
		},
		RemoteServer: config.ClusterDefinition{
			Connection: config.TransportInfo{
				ConnectionType: config.ConnTypeTCP,
				TcpServer: config.TCPTLSInfo{
					ConnectionString: localToRemoteServer,
				},
				TcpClient: config.TCPTLSInfo{
					ConnectionString: remoteServer,
				},
			},
		},
	}
}

func makeMuxClusterConfig(name string, client config.ConnectionType, localTemporal string, outboundServer string, muxAddr string,
	edits ...func(connConfig *config.ClusterConnConfig)) config.ClusterConnConfig {
	cc := config.ClusterConnConfig{
		Name: name,
		LocalServer: config.ClusterDefinition{
			Connection: config.TransportInfo{
				ConnectionType: config.ConnTypeTCP,
				TcpServer: config.TCPTLSInfo{
					ConnectionString: outboundServer,
				},
				TcpClient: config.TCPTLSInfo{
					ConnectionString: localTemporal,
				},
			},
		},
		RemoteServer: config.ClusterDefinition{
			Connection: config.TransportInfo{
				ConnectionType: client,
				MuxAddressInfo: config.TCPTLSInfo{
					ConnectionString: muxAddr,
					// No TLS
				},
			},
		},
	}
	for _, f := range edits {
		f(&cc)
	}
	return cc
}

func makeEchoServer(name string, listenAddress string, logger log.Logger) *testservices.TemporalServerWithListen {
	logger.Info("Starting echo server", tag.NewStringTag("name", name), tag.Address(listenAddress))
	return testservices.NewTemporalAPIServer(name,
		testservices.NewEchoAdminService(name, nil, logger),
		testservices.NewEchoWorkflowService(name, logger),
		nil, listenAddress, logger)
}

func newPairedLocalClusterConnection(t *testing.T, isMux bool, logger log.Logger) *pairedLocalClusterConnection {
	a := getDynamicPlccAddresses(t)

	localTemporal := makeEchoServer("local", a.localTemporalAddr, logger)
	remoteTemporal := makeEchoServer("remote", a.remoteTemporalAddr, logger)

	var localCC, remoteCC *ClusterConnection
	var cancelLocalCC, cancelRemoteCC context.CancelFunc
	var err error
	if !isMux {
		var localCtx context.Context
		localCtx, cancelLocalCC = context.WithCancel(t.Context())
		localCC, err = NewClusterConnection(localCtx, makeTCPClusterConfig("TCP-only Connection Local Proxy",
			a.localTemporalAddr, a.localProxyInbound, a.localProxyOutbound, a.remoteProxyInbound), logger)
		require.NoError(t, err)

		var remoteCtx context.Context
		remoteCtx, cancelRemoteCC = context.WithCancel(t.Context())
		remoteCC, err = NewClusterConnection(remoteCtx, makeTCPClusterConfig("TCP-only Connection Remote Proxy",
			a.remoteTemporalAddr, a.remoteProxyInbound, a.remoteProxyOutbound, a.localProxyInbound), logger)
		require.NoError(t, err)
	} else {
		var localCtx context.Context
		localCtx, cancelLocalCC = context.WithCancel(t.Context())
		localCC, err = NewClusterConnection(localCtx, makeMuxClusterConfig("Mux Connection Local Establishing Proxy",
			config.ConnTypeMuxClient, a.localTemporalAddr, a.localProxyOutbound, a.remoteProxyInbound), logger)
		require.NoError(t, err)

		var remoteCtx context.Context
		remoteCtx, cancelRemoteCC = context.WithCancel(t.Context())
		remoteCC, err = NewClusterConnection(remoteCtx, makeMuxClusterConfig("Mux Connection Remote Receiving Proxy",
			config.ConnTypeMuxServer, a.remoteTemporalAddr, a.remoteProxyOutbound, a.remoteProxyInbound), logger)
		require.NoError(t, err)
	}
	clientFromLocal, err := grpc.NewClient(a.localProxyOutbound, grpcutil.MakeDialOptions(nil, metrics.GetStandardGRPCClientInterceptor("outbound-local"))...)
	require.NoError(t, err)
	clientFromRemote, err := grpc.NewClient(a.remoteProxyOutbound, grpcutil.MakeDialOptions(nil, metrics.GetStandardGRPCClientInterceptor("outbound-remote"))...)
	require.NoError(t, err)
	return &pairedLocalClusterConnection{
		localTemporal:    localTemporal,
		remoteTemporal:   remoteTemporal,
		localCC:          localCC,
		cancelLocalCC:    cancelLocalCC,
		remoteCC:         remoteCC,
		cancelRemoteCC:   cancelRemoteCC,
		clientFromLocal:  clientFromLocal,
		clientFromRemote: clientFromRemote,
		addresses:        a,
	}
}

func TestTCPClusterConnection(t *testing.T) {
	logger := log.NewTestLogger()
	plcc := newPairedLocalClusterConnection(t, false, logger)
	plcc.StartAll(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := adminservice.NewAdminServiceClient(plcc.clientFromLocal).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, "remote-EchoAdminService", resp.ClusterName, "Should see remote EchoAdminService from the local outbound")
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	resp, err = adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, "local-EchoAdminService", resp.ClusterName, "Should see local EchoAdminService from the remote outbound")
	cancel()
}

func TestMuxClusterConnection(t *testing.T) {
	logger := log.NewTestLogger()
	plcc := newPairedLocalClusterConnection(t, true, logger)
	plcc.StartAll(t)
	t.Log("Started plcc")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	resp, err := adminservice.NewAdminServiceClient(plcc.clientFromLocal).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, "remote-EchoAdminService", resp.ClusterName, "Should see remote EchoAdminService from the local outbound")
	t.Log("Called remote!")
	cancel()
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	resp, err = adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, "local-EchoAdminService", resp.ClusterName, "Should see local EchoAdminService from the remote outbound")
	t.Log("Finished!")
	cancel()
}

func TestMuxCCFailover(t *testing.T) {
	logger := log.NewTestLogger()
	plcc := newPairedLocalClusterConnection(t, true, logger)
	plcc.StartAll(t)

	plcc.cancelRemoteCC()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	_, err := adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.Error(t, err)
	cancel()
	newConnection, err := NewClusterConnection(t.Context(),
		makeMuxClusterConfig("newRemoteMux", config.ConnTypeMuxServer, plcc.addresses.remoteTemporalAddr, plcc.addresses.remoteProxyOutbound, plcc.addresses.remoteProxyInbound,
			func(cc *config.ClusterConnConfig) { cc.RemoteServer.Connection.MuxCount = 5 }), logger)
	require.NoError(t, err)
	newConnection.Start()
	// Wait for localCC's client retry...
	timeout := time.Now().Add(2 * time.Second)
	var resp *adminservice.DescribeClusterResponse
	err = errors.New("didn't complete a single request")
	for time.Now().Before(timeout) {
		ctx, cancel = context.WithTimeout(t.Context(), time.Second)
		resp, err = adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
		if err == nil {
			break
		}
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "local-EchoAdminService", resp.ClusterName, "Local cluster connection should have reconnected")
	cancel()
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	resp, err = adminservice.NewAdminServiceClient(plcc.clientFromLocal).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoError(t, err)
	require.Equal(t, "remote-EchoAdminService", resp.ClusterName, "Local cluster connection should have reconnected")
	cancel()
}
