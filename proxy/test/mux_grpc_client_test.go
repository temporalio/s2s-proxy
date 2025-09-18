package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

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
	s := &muxSession{}
	s.addr = listener.Addr().String()
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		var err error
		s.serverConn, err = listener.Accept()
		require.NoError(t, err)
		s.serverMux, err = yamux.Server(s.serverConn, nil)
		require.NoError(t, err)
		wg.Done()
	}()
	go func() {
		var err error
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
			if err != nil && !errors.Is(err, yamux.ErrSessionShutdown) {
				require.Failf(t, "Error during Serve %s", err.Error())
			}
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

func (s *testScenario) close() {
	_ = s.listener.Close()
	for _, mux := range s.muxes {
		_ = mux.serverMux.Close()
		_ = mux.clientMux.Close()
		_ = mux.serverConn.Close()
		_ = mux.clientConn.Close()
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
	scenario.close()
}

// Using a single ClientConn object and naively swapping in a round-robin Dialer does not work, because
// the connection is cached.
func TestSingleClientCustomDialer(t *testing.T) {
	scenario := newTestScenario(t)
	counter := &atomic.Uint32{}
	counter.Store(0)
	superClientConn, err := grpc.NewClient("passthrough:unused",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			//t.Log("Dialed addr", addr)
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
	scenario.close()
}

// We require a custom dialer at all times, because our dialer needs to return the right mux connection.
// The dialer function only takes a string address and a context. The Balancer and Resolver don't have access
// to the context passed to Dial, which means our only real input is the address. Luckily, we have the local context
// the Dialer was constructed in, which means we can use a map from address -> mux-conn. The manual resolver will be
// notified with one endpoint per mux map key, and those map keys will arrive at the dialer.
func TestSingleClientCustomResolverAndDialer(t *testing.T) {
	scenario := newTestScenario(t)
	manualResolver := manual.NewBuilderWithScheme("multimux")
	// There is a note in resolver.State that mentions how Addresses and Endpoints work:
	//  //If a resolver sets Addresses but does not set Endpoints, one Endpoint
	//	// will be created for each Address before the State is passed to the LB
	//	// policy.  The BalancerAttributes of each entry in Addresses will be set
	//	// in Endpoints.Attributes, and be cleared in the Endpoint's Address's
	//	// BalancerAttributes.
	//	//
	//	// Soon, Addresses will be deprecated and replaced fully by Endpoints.
	//addresses := []resolver.Address{
	//	{Addr: "0", ServerName: "mux0", Attributes: attributes.New("mux", scenario.muxes[0].clientMux)},
	//	{Addr: "1", ServerName: "mux1", Attributes: attributes.New("mux", scenario.muxes[1].clientMux)},
	//	{Addr: "2", ServerName: "mux2", Attributes: attributes.New("mux", scenario.muxes[2].clientMux)},
	//	{Addr: "3", ServerName: "mux3", Attributes: attributes.New("mux", scenario.muxes[3].clientMux)},
	//	{Addr: "4", ServerName: "mux4", Attributes: attributes.New("mux", scenario.muxes[4].clientMux)},
	//}
	manualResolver.InitialState(resolver.State{
		//Addresses: addresses,
		Endpoints: []resolver.Endpoint{
			{Addresses: []resolver.Address{{Addr: "0", ServerName: "mux0", Attributes: attributes.New("mux", scenario.muxes[0].clientMux)}}},
			{Addresses: []resolver.Address{{Addr: "1", ServerName: "mux1", Attributes: attributes.New("mux", scenario.muxes[1].clientMux)}}},
			{Addresses: []resolver.Address{{Addr: "2", ServerName: "mux2", Attributes: attributes.New("mux", scenario.muxes[2].clientMux)}}},
			{Addresses: []resolver.Address{{Addr: "3", ServerName: "mux3", Attributes: attributes.New("mux", scenario.muxes[3].clientMux)}}},
			{Addresses: []resolver.Address{{Addr: "4", ServerName: "mux4", Attributes: attributes.New("mux", scenario.muxes[4].clientMux)}}},
		},
	})
	connSeen := []atomic.Bool{{}, {}, {}, {}, {}}
	superClientConn, err := grpc.NewClient("multimux://unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithResolvers(manualResolver),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			i, err := strconv.Atoi(addr)
			//t.Log("Dialed addr", addr)
			require.NoError(t, err)
			connSeen[i].Store(true)
			return scenario.muxes[i].clientMux.Open()
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	for range 5 {
		resp, err := adminservice.NewAdminServiceClient(superClientConn).DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		t.Log("Got cluster", resp.ClusterName)
	}
	for i := range 5 {
		assert.Truef(t, connSeen[i].Load(), "Did not see connection on %d", i)
	}
	time.Sleep(time.Second)
	scenario.close()
}
