package adminplane

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/config"
)

// answer stands in for a response message.
// Serve is generic.
// None of this needs a proto.
type answer struct {
	ID  string
	Sum int
}

// fakeConn is the connection the fake dialer hands back.
// It carries the address it was opened for.
// A member's answer is distinguishable that way without a transport.
type fakeConn struct{ address string }

func (fakeConn) Invoke(context.Context, string, any, any, ...grpc.CallOption) error { return nil }
func (fakeConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}

type failingDiscovery struct{}

func (failingDiscovery) Provider() string { return config.DiscoveryDNS }
func (failingDiscovery) Discover(context.Context) ([]string, error) {
	return nil, errors.New("no such host")
}

// dialRecorder is a fake dialer that records what was dialed and how much of it happened at once.
type dialRecorder struct {
	mu       sync.Mutex
	dialed   []string
	inFlight int
	peak     int
	fail     map[string]error
	hold     time.Duration
}

func (d *dialRecorder) dial(_ context.Context, address string) (grpc.ClientConnInterface, func(), error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, address)
	d.inFlight++
	if d.inFlight > d.peak {
		d.peak = d.inFlight
	}
	err := d.fail[address]
	d.mu.Unlock()

	release := func() {
		d.mu.Lock()
		d.inFlight--
		d.mu.Unlock()
	}
	if err != nil {
		release()
		return nil, func() {}, err
	}
	if d.hold > 0 {
		time.Sleep(d.hold)
	}
	return fakeConn{address: address}, release, nil
}

func (d *dialRecorder) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.dialed)
}

func (d *dialRecorder) peakConcurrency() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peak
}

// handlers answer as whichever address was dialed.
// Each member is identifiable in the result.
func handlers(selfID string) Handlers[struct{}, answer] {
	return Handlers[struct{}, answer]{
		Self: func(context.Context, struct{}) answer { return answer{ID: selfID, Sum: 1} },
		Call: func(_ context.Context, cc grpc.ClientConnInterface, _ struct{}) (answer, error) {
			return answer{ID: cc.(fakeConn).address, Sum: 1}, nil
		},
		Merge: func(self Result[answer], members []Result[answer], _ Roster) answer {
			out := answer{ID: self.Value.ID, Sum: self.Value.Sum}
			for _, m := range members {
				out.Sum += m.Value.Sum
			}
			return out
		},
		ID:   func(a answer) string { return a.ID },
		View: func(a answer, _ ServerOptions) answer { return a },
	}
}

func planeFor(memberID string, addresses []string) (*Plane, *dialRecorder) {
	rec := &dialRecorder{}
	return &Plane{
		Discovery: NewStaticDiscovery(addresses),
		Dial:      rec.dial,
		MemberID:  memberID,
	}, rec
}

// groupCtx is the context the interceptor produces for a group-scoped operator call.
func groupCtx() context.Context {
	ctx := context.WithValue(context.Background(), optionsKey{}, ServerOptions{Role: RoleOperator})
	return context.WithValue(ctx, scopeKey{}, ScopeGroup)
}

// Serve takes this process's own answer before fanning out.
// Every way out of fanOut already has one responder.
//
// The first case is the shipped default.
// The chart turns the operator listener on and leaves the peer block off.
// That leaves Dial nil.
func TestFanOutAlwaysCountsThisProcess(t *testing.T) {
	cases := []struct {
		name  string
		plane *Plane
	}{
		{name: "no peer configuration", plane: &Plane{Discovery: NoDiscovery(), MemberID: "pod-a"}},
		{name: "discovery failed", plane: &Plane{Discovery: failingDiscovery{}, Dial: (&dialRecorder{}).dial, MemberID: "pod-a"}},
		{name: "nothing discovered", plane: &Plane{Discovery: NewStaticDiscovery(nil), Dial: (&dialRecorder{}).dial, MemberID: "pod-a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, roster := fanOut(groupCtx(), c.plane, struct{}{}, handlers("pod-a"))
			require.Equal(t, 1, roster.Responding,
				"this process answered, so a roster saying nobody did is self-contradictory")
		})
	}
}

// A budget that has already run out must not read as a clean, empty roster.
func TestFanOutReportsWhenThereWasNoTimeToAsk(t *testing.T) {
	plane, rec := planeFor("pod-a", []string{"10.0.0.1:9234", "10.0.0.2:9234"})

	// Already past its deadline.
	// Clamp returns non-positive at the first stage.
	ctx, cancel := context.WithTimeout(groupCtx(), -time.Second)
	defer cancel()

	members, roster := fanOut(ctx, plane, struct{}{}, handlers("pod-a"))

	require.Empty(t, members)
	require.Equal(t, 1, roster.Responding)
	require.Equal(t, 0, rec.count(), "no member should be dialed once the budget is gone")
}

func TestFanOutReportsDiscoveryFailure(t *testing.T) {
	plane := &Plane{Discovery: failingDiscovery{}, Dial: (&dialRecorder{}).dial, MemberID: "pod-a"}

	_, roster := fanOut(groupCtx(), plane, struct{}{}, handlers("pod-a"))

	require.Len(t, roster.Unreachable, 1)
	require.Contains(t, roster.Unreachable[0].Message, "discovery failed")
	// No address: the failure was enumerating members, not reaching one.
	require.Empty(t, roster.Unreachable[0].Address)
	require.Equal(t, int32(codes.Unavailable), roster.Unreachable[0].Code)
}

// Every discovered address is dialed, this pod's own included.
// A DNS record carries no identity.
// Self is recognized afterwards, by the id in its reply.
func TestFanOutDropsOurOwnReply(t *testing.T) {
	plane, rec := planeFor("10.0.0.1:9234", []string{"10.0.0.1:9234", "10.0.0.2:9234"})

	members, roster := fanOut(groupCtx(), plane, struct{}{}, handlers("10.0.0.1:9234"))

	require.Equal(t, 2, rec.count(), "self is not recognizable before the dial, so it is dialed too")
	require.Len(t, members, 1, "our own reply is dropped, or every count doubles")
	require.Equal(t, "10.0.0.2:9234", members[0].ID)
	require.Equal(t, 2, roster.Responding)
	require.Equal(t, 2, roster.Discovered)
}

// Without an ID function there is nothing to compare.
// Self cannot be recognized and is counted twice.
// Serve does not reject that today.
// This pins the cost so it is visible.
func TestFanOutCannotDropOurOwnReplyWithoutAnIDFunc(t *testing.T) {
	plane, _ := planeFor("10.0.0.1:9234", []string{"10.0.0.1:9234", "10.0.0.2:9234"})

	h := handlers("10.0.0.1:9234")
	h.ID = nil

	members, roster := fanOut(groupCtx(), plane, struct{}{}, h)

	require.Len(t, members, 2)
	require.Equal(t, 3, roster.Responding, "self is counted once locally and once over its own listener")
}

func TestFanOutRecordsAMemberThatCannotBeDialed(t *testing.T) {
	plane, rec := planeFor("10.0.0.1:9234", []string{"10.0.0.1:9234", "10.0.0.9:9234"})
	rec.fail = map[string]error{"10.0.0.9:9234": status.Error(codes.Unavailable, "connection refused")}

	members, roster := fanOut(groupCtx(), plane, struct{}{}, handlers("10.0.0.1:9234"))

	require.Empty(t, members, "the only address that answered was our own, which is dropped")
	require.Len(t, roster.Unreachable, 1)
	require.Equal(t, "10.0.0.9:9234", roster.Unreachable[0].Address)
	require.Equal(t, int32(codes.Unavailable), roster.Unreachable[0].Code)
	require.Equal(t, 1, roster.Responding)
}

// A wrong DNS name should cost a bounded number of simultaneous connections, not one per pod.
func TestFanOutCapsConcurrentDials(t *testing.T) {
	addresses := make([]string, 0, 64)
	for i := range 64 {
		addresses = append(addresses, "10.0.0."+strconv.Itoa(i)+":9234")
	}
	plane, rec := planeFor("pod-a", addresses)
	rec.hold = 20 * time.Millisecond

	fanOut(groupCtx(), plane, struct{}{}, handlers("pod-a"))

	require.Equal(t, 64, rec.count())
	require.LessOrEqual(t, rec.peakConcurrency(), maxConcurrentMembers)
}

// Serve refuses to answer rather than trusting that the handler was assembled correctly.
// It also refuses rather than trusting that a listener installed the interceptor that decides its policy.
func TestServeRefusesToAnswerWhatItCannotPolice(t *testing.T) {
	withOptions := func(o ServerOptions) context.Context {
		ctx := context.WithValue(context.Background(), optionsKey{}, o)
		return context.WithValue(ctx, scopeKey{}, ScopeMember)
	}
	counterparty := ServerOptions{Role: RoleCounterparty, ConnectionName: "cluster-b"}

	cases := []struct {
		name  string
		ctx   context.Context
		muted func(h *Handlers[struct{}, answer])
	}{
		{
			// A response bound for another organization must have been narrowed on purpose.
			// Narrowing by whichever fields the endpoint happened to populate is not on purpose.
			name:  "counterparty with no View",
			ctx:   withOptions(counterparty),
			muted: func(h *Handlers[struct{}, answer]) { h.View = nil },
		},
		{
			// Role is stamped by the interceptor.
			// Reaching here without one means the listener registered the service but not the policy.
			// The zero value must not be the one that skips narrowing.
			name:  "no listener policy at all",
			ctx:   withOptions(ServerOptions{}),
			muted: func(*Handlers[struct{}, answer]) {},
		},
		{
			name:  "a required handler is missing",
			ctx:   withOptions(ServerOptions{Role: RoleOperator}),
			muted: func(h *Handlers[struct{}, answer]) { h.Merge = nil },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := handlers("pod-a")
			c.muted(&h)
			_, err := Serve(c.ctx, &Plane{Discovery: NoDiscovery(), MemberID: "pod-a"}, struct{}{}, h)
			require.Error(t, err)
			require.Equal(t, codes.Internal, status.Code(err))
		})
	}
}

// Forwarding needs a router.
// Without one the call is refused rather than reported as a member that did not answer.
func TestForwardWithoutARouter(t *testing.T) {
	ctx := context.WithValue(context.Background(), optionsKey{}, ServerOptions{Role: RoleOperator})
	ctx = context.WithValue(ctx, scopeKey{}, ScopeGroup)
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(MDTarget, "cluster-b"))

	_, err := Serve(ctx, &Plane{Discovery: NoDiscovery(), MemberID: "pod-a"}, struct{}{}, handlers("pod-a"))
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
