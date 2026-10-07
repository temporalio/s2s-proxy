package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// recordingConn is a [closableClientConn] that records the calls that reach it
// instead of making them.
type recordingConn struct {
	invoked  []string
	streamed []string
}

func (c *recordingConn) Invoke(_ context.Context, method string, _, _ any, _ ...grpc.CallOption) error {
	c.invoked = append(c.invoked, method)
	return nil
}

func (c *recordingConn) NewStream(_ context.Context, _ *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	c.streamed = append(c.streamed, method)
	return nil, nil
}

func (c *recordingConn) Close() error       { return nil }
func (c *recordingConn) Describe() string   { return "recordingConn" }
func (c *recordingConn) CanMakeCalls() bool { return true }

func TestInterceptedConnRunsInterceptorAroundInvoke(t *testing.T) {
	var events []string
	cc := &recordingConn{}
	conn := interceptedConn{
		ClientConnInterface: cc,
		intercept: func(ctx context.Context, method string, req, reply any, _ *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			events = append(events, "before "+method)
			err := invoker(ctx, method, req, reply, nil, opts...)
			events = append(events, "after "+method)
			return err
		},
	}

	require.NoError(t, conn.Invoke(t.Context(), "/svc/Call", nil, nil))
	require.Equal(t, []string{"before /svc/Call", "after /svc/Call"}, events)
	require.Equal(t, []string{"/svc/Call"}, cc.invoked)
}

func TestInterceptedConnRunsStreamInterceptorAroundNewStream(t *testing.T) {
	var events []string
	cc := &recordingConn{}
	conn := interceptedConn{
		ClientConnInterface: cc,
		interceptStream: func(ctx context.Context, desc *grpc.StreamDesc, _ *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			events = append(events, "before "+method)
			cs, err := streamer(ctx, desc, nil, method, opts...)
			events = append(events, "after "+method)
			return cs, err
		},
	}

	_, err := conn.NewStream(t.Context(), &grpc.StreamDesc{}, "/svc/Stream")
	require.NoError(t, err)
	require.Equal(t, []string{"before /svc/Stream", "after /svc/Stream"}, events)
	require.Equal(t, []string{"/svc/Stream"}, cc.streamed)
}

func TestInterceptedConnWithoutStreamInterceptorPassesStreamsThrough(t *testing.T) {
	cc := &recordingConn{}
	conn := interceptedConn{ClientConnInterface: cc}

	_, err := conn.NewStream(t.Context(), &grpc.StreamDesc{}, "/svc/Stream")
	require.NoError(t, err)
	require.Equal(t, []string{"/svc/Stream"}, cc.streamed)
}
