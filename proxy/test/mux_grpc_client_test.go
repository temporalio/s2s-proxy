package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporalio/s2s-proxy/common"
)

type muxedServer struct {
	server  *grpc.Server
	session *muxSession
}

type muxedClient struct {
	session    *muxSession
	clientConn *grpc.ClientConn
	client     adminservice.AdminServiceClient
}

type muxSession struct {
	addr       string
	serverConn net.Conn
	clientConn net.Conn
	serverMux  *yamux.Session
	clientMux  *yamux.Session
}

func newMuxSession(t *testing.T, listener net.Listener) *muxSession {
	var err error
	s := &muxSession{}
	s.addr = listener.Addr().String()
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		s.serverConn, err = listener.Accept()
		require.NoError(t, err)
		s.serverMux, err = yamux.Server(s.serverConn, nil)
		require.NoError(t, err)
		wg.Done()
	}()
	go func() {
		s.clientConn, err = net.Dial("tcp", s.addr)
		require.NoError(t, err)
		s.clientMux, err = yamux.Client(s.clientConn, nil)
		require.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()
	return s
}

type testScenario struct {
	muxes    []*muxSession
	servers  []*muxedServer
	clients  []*muxedClient
	listener net.Listener
}

func newTestScenario(t *testing.T) *testScenario {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	servers := make([]*muxedServer, 10)
	clients := make([]*muxedClient, 10)
	muxes := make([]*muxSession, 10)
	for i := range 10 {
		muxes[i] = newMuxSession(t, listener)
		servers[i] = &muxedServer{
			server:  grpc.NewServer(),
			session: muxes[i],
		}
		serviceName := fmt.Sprintf("adminService on mux %d", i)
		eas := &echoAdminService{
			serviceName: serviceName,
			logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(muxes[i].addr)),
			namespaces:  map[string]bool{"hello": true, "world": true},
			payloadSize: defaultPayloadSize,
		}
		adminservice.RegisterAdminServiceServer(servers[i].server, eas)
		go func() {
			err := servers[i].server.Serve(servers[i].session.serverMux)
			require.NoError(t, err)
		}()
		session := servers[i].session
		clientConn, err := grpc.NewClient("passthrough:unused",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return session.clientMux.Open()
			}),
			grpc.WithDisableServiceConfig(),
		)
		require.NoError(t, err)
		clients[i] = &muxedClient{
			session:    muxes[i],
			clientConn: clientConn,
			client:     adminservice.NewAdminServiceClient(clientConn),
		}
	}
	return &testScenario{
		muxes:    muxes,
		servers:  servers,
		clients:  clients,
		listener: listener,
	}
}

// Using separate ClientConn objects created using grpc.NewClientConn "just works", but the clients are completely
// separate and do not load-balance. We have to do the work of swapping out the client objects in the calling code.
func TestMultiClientMultiServer(t *testing.T) {
	scenario := newTestScenario(t)
	for i, muxClient := range scenario.clients {
		resp, err := muxClient.client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("adminService on mux %d", i), resp.ClusterName, "Invalid or missing cluster name")
	}
}

// Using a single ClientConn object and naively swapping in a round-robin Dialer does not work, because
// the connection is cached.
func TestSingleClientCustomDialer(t *testing.T) {
	scenario := newTestScenario(t)
	counter := &atomic.Uint32{}
	counter.Store(0)
	superClientConn, err := grpc.NewClient("passthrough:unused",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			t.Log("Dialed addr", addr)
			return scenario.muxes[counter.Add(1)%10].clientMux.Open()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	for range 10 {
		resp, err := adminservice.NewAdminServiceClient(superClientConn).DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		require.Equal(t, "adminService on mux 1", resp.ClusterName)
	}
}
