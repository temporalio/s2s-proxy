package endtoendtest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/endtoendtest/testservices"
)

// This file sets up N echo servers listening on a local pipe. You have access to both the Server and the client
// side of the pipe, and NewYamuxGRPCScenario will start echoadminservices for each MuxSession.

type MuxedServer struct {
	Server           *grpc.Server
	Session          *MuxSession
	EchoAdminService *testservices.EchoAdminService
}

type MuxedClient struct {
	Session    *MuxSession
	ClientConn *grpc.ClientConn
	Client     adminservice.AdminServiceClient
}

type MuxSession struct {
	Addr       string
	ServerConn net.Conn
	ClientConn net.Conn
	ServerMux  *yamux.Session
	ClientMux  *yamux.Session
}

func NewMuxSession(t *testing.T, listener net.Listener) *MuxSession {
	s := &MuxSession{}
	s.Addr = listener.Addr().String()
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		var err error
		s.ServerConn, err = listener.Accept()
		require.NoError(t, err)
		s.ServerMux, err = yamux.Server(s.ServerConn, nil)
		require.NoError(t, err)
		wg.Done()
	}()
	go func() {
		var err error
		s.ClientConn, err = net.Dial("tcp", s.Addr)
		require.NoError(t, err)
		s.ClientMux, err = yamux.Client(s.ClientConn, nil)
		require.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()
	return s
}

type TestScenario struct {
	Muxes    []*MuxSession
	Servers  []*MuxedServer
	Clients  []*MuxedClient
	Listener net.Listener
}

func NewYamuxGRPCScenario(t *testing.T, size int, temporalLogger log.Logger) *TestScenario {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	servers := make([]*MuxedServer, size)
	clients := make([]*MuxedClient, size)
	muxes := make([]*MuxSession, size)
	for i := range size {
		muxes[i] = NewMuxSession(t, listener)
		servers[i] = &MuxedServer{
			Server:  grpc.NewServer(),
			Session: muxes[i],
		}
		serviceName := fmt.Sprintf("adminService on mux %d", i)
		eas := &testservices.EchoAdminService{
			ServiceName: serviceName,
			Logger:      log.With(temporalLogger, common.ServiceTag(serviceName), tag.Address(muxes[i].Addr)),
			Namespaces:  map[string]bool{"hello": true, "world": true},
			PayloadSize: defaultPayloadSize,
		}
		servers[i].EchoAdminService = eas
		adminservice.RegisterAdminServiceServer(servers[i].Server, eas)
		go func() {
			err := servers[i].Server.Serve(servers[i].Session.ServerMux)
			if err != nil && !errors.Is(err, yamux.ErrSessionShutdown) {
				require.Failf(t, "Error during Serve %s", err.Error())
			}
		}()
		session := servers[i].Session
		clientConn, err := grpc.NewClient("passthrough:unused",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return session.ClientMux.Open()
			}),
			grpc.WithDisableServiceConfig(),
		)
		require.NoError(t, err)
		clients[i] = &MuxedClient{
			Session:    muxes[i],
			ClientConn: clientConn,
			Client:     adminservice.NewAdminServiceClient(clientConn),
		}
	}
	return &TestScenario{
		Muxes:    muxes,
		Servers:  servers,
		Clients:  clients,
		Listener: listener,
	}
}

func (s *TestScenario) CloseMux(i int) {
	_ = s.Muxes[i].ServerMux.Close()
	_ = s.Muxes[i].ClientMux.Close()
	_ = s.Muxes[i].ServerConn.Close()
	_ = s.Muxes[i].ClientConn.Close()
}

func (s *TestScenario) Close() {
	_ = s.Listener.Close()
	for i := range s.Muxes {
		s.CloseMux(i)
	}
}
