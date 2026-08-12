package adminplane

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// maxAdminRecvBytes caps a response from another proxy.
// Fan-out multiplies whatever one member sends.
// The replication dial options raise this limit far higher than an admin response should ever need.
const maxAdminRecvBytes = 4 << 20

// DialOnce opens a connection for a single admin call.
// The returned function closes it.
//
// It is deliberately not pooled and deliberately not tied to a process lifetime.
// Pod addresses change on every rollout.
// A pool would need eviction.
// A lifetime-scoped cleanup hook would accumulate one registration per member per call for as long as the process runs.
func DialOnce(address string, tlsConfig *tls.Config) (grpc.ClientConnInterface, func(), error) {
	creds := insecure.NewCredentials()
	if tlsConfig != nil {
		creds = credentials.NewTLS(tlsConfig)
	}
	cc, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(creds),
		// Deliberately fail fast, the default.
		// A connecting channel already waits.
		// A healthy but cold peer still succeeds.
		// What fails immediately is a channel that reached TRANSIENT_FAILURE.
		// Its cause is the answer the caller wanted.
		// WaitForReady would block through that until the budget expired and report every dead peer as a timeout.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxAdminRecvBytes)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{PermitWithoutStream: false}),
	)
	if err != nil {
		return nil, func() {}, err
	}
	return cc, func() { _ = cc.Close() }, nil
}

// Server is a gRPC server on its own listener, stopped when its lifetime ends.
//
// Serve is called once.
// Unlike the replication servers there is no retry loop.
// A listener that stops accepting will not start again by being asked more often.
// Retrying every second would turn one failure into an unbounded stream of log lines.
type Server struct {
	name     string
	lifetime context.Context
	listener net.Listener
	server   *grpc.Server
	logger   log.Logger
}

// NewServer prepares a server.
func NewServer(name string, lifetime context.Context, listener net.Listener, server *grpc.Server, logger log.Logger) *Server {
	return &Server{name: name, lifetime: lifetime, listener: listener, server: server, logger: logger}
}

func (s *Server) Start() {
	context.AfterFunc(s.lifetime, func() {
		s.server.GracefulStop()
		_ = s.listener.Close()
	})
	go func() {
		s.logger.Info("Starting admin gRPC server",
			tag.Name(s.name), tag.Address(s.listener.Addr().String()))
		err := s.server.Serve(s.listener)
		if s.lifetime.Err() != nil || err == nil || errors.Is(err, grpc.ErrServerStopped) {
			s.logger.Info("Admin gRPC server stopped", tag.Name(s.name))
			return
		}
		s.logger.Error("Admin gRPC server failed",
			tag.Name(s.name), tag.Address(s.listener.Addr().String()), tag.Error(err))
	}()
}
