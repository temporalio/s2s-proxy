package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

type clientTransportWaitContextKey struct{}

type clientTransportWaitState struct {
	direction string
	pool      string
	service   string
	method    string

	mu       sync.Mutex
	started  time.Time
	grpcType string
	waiting  bool
	finished bool
}

// NewClientTransportWaitStatsHandler measures how long a client RPC waits
// before gRPC emits its outbound headers. grpc-go emits Begin before its picker
// runs and OutHeader only after the selected transport admits a new stream, so
// this interval includes resolver, picker, connection, and transport admission
// wait but excludes the upstream response. An RPC that ends first is classified
// separately so transport starvation is distinguishable from a slow upstream.
func NewClientTransportWaitStatsHandler(direction string, pool string) stats.Handler {
	if pool == "" {
		pool = "shared"
	}
	return &clientTransportWaitStatsHandler{direction: direction, pool: pool}
}

type clientTransportWaitStatsHandler struct {
	direction string
	pool      string
}

func (h *clientTransportWaitStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	service, method := splitFullMethod(info.FullMethodName)
	state := &clientTransportWaitState{
		direction: h.direction,
		pool:      h.pool,
		service:   service,
		method:    method,
	}
	return context.WithValue(ctx, clientTransportWaitContextKey{}, state)
}

func (*clientTransportWaitStatsHandler) HandleRPC(ctx context.Context, rpcStats stats.RPCStats) {
	state, ok := ctx.Value(clientTransportWaitContextKey{}).(*clientTransportWaitState)
	if !ok {
		return
	}

	switch event := rpcStats.(type) {
	case *stats.Begin:
		state.begin(event)
	case *stats.OutHeader:
		state.finish("ready")
	case *stats.End:
		state.finish(outcomeBeforeHeader(event.Error))
	}
}

func (*clientTransportWaitStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (*clientTransportWaitStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

func (s *clientTransportWaitState) begin(event *stats.Begin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waiting || s.finished {
		return
	}
	s.started = time.Now()
	s.grpcType = grpcType(event.IsClientStream, event.IsServerStream)
	s.waiting = true
	GRPCClientTransportWaiting.WithLabelValues(s.direction, s.pool).Inc()
}

func (s *clientTransportWaitState) finish(outcome string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.waiting || s.finished {
		return
	}
	s.finished = true
	s.waiting = false
	GRPCClientTransportWaiting.WithLabelValues(s.direction, s.pool).Dec()
	GRPCClientTimeToOutHeader.WithLabelValues(
		s.direction,
		s.pool,
		s.grpcType,
		s.service,
		s.method,
		outcome,
	).Observe(time.Since(s.started).Seconds())
}

func splitFullMethod(fullMethod string) (string, string) {
	trimmed := strings.TrimPrefix(fullMethod, "/")
	service, method, found := strings.Cut(trimmed, "/")
	if !found {
		return "unknown", trimmed
	}
	return service, method
}

func grpcType(clientStream, serverStream bool) string {
	switch {
	case clientStream && serverStream:
		return "bidi_stream"
	case clientStream:
		return "client_stream"
	case serverStream:
		return "server_stream"
	default:
		return "unary"
	}
}

func outcomeBeforeHeader(err error) string {
	switch status.Code(err) {
	case codes.DeadlineExceeded:
		return "deadline_exceeded_before_header"
	case codes.Canceled:
		return "canceled_before_header"
	default:
		return "ended_before_header"
	}
}
