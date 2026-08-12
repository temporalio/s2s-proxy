package adminplane

import (
	"context"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Default budgets. Every one is clamped to the caller's deadline when there is one.
const (
	DefaultDiscoverBudget = 1 * time.Second
	DefaultMemberBudget   = 2 * time.Second
	DefaultForwardBudget  = 5 * time.Second

	// deadlineSlack leaves room to build a response after the last member call returns.
	deadlineSlack = 250 * time.Millisecond

	// maxConcurrentMembers bounds simultaneous dials during fan-out.
	maxConcurrentMembers = 16
)

// Budgets are the per-stage time limits for a call.
type Budgets struct {
	Discover time.Duration
	Member   time.Duration
	Forward  time.Duration
}

func (b Budgets) withDefaults() Budgets {
	if b.Discover <= 0 {
		b.Discover = DefaultDiscoverBudget
	}
	if b.Member <= 0 {
		b.Member = DefaultMemberBudget
	}
	if b.Forward <= 0 {
		b.Forward = DefaultForwardBudget
	}
	return b
}

// Clamp shortens want to fit the context's deadline.
// It never extends it.
//
// The obvious formulation, min(want, time.Until(deadline)-slack), is wrong for a context with no deadline.
// Deadline reports the zero time there.
// time.Until of the zero time is hugely negative.
// Every call would be given a budget in the past.
func Clamp(ctx context.Context, want time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return want
	}
	if remaining := time.Until(deadline) - deadlineSlack; remaining < want {
		return remaining
	}
	return want
}

// PeerRouter resolves a cluster connection name to a connection to that connection's peer proxy.
type PeerRouter interface {
	// PeerConn returns a client for the named cluster connection, or a gRPC status error.
	// InvalidArgument means no such connection is configured.
	// FailedPrecondition means it exists but cannot currently carry a call.
	PeerConn(name string) (grpc.ClientConnInterface, error)
}

// DialFunc opens a connection to a peer admin listener.
// The returned function releases it.
type DialFunc func(ctx context.Context, address string) (grpc.ClientConnInterface, func(), error)

// Plane holds everything an admin endpoint needs to answer beyond its own process.
type Plane struct {
	Discovery Discovery
	Dial      DialFunc
	Peers     PeerRouter
	// MemberID identifies this process in its own answers.
	// It is also how this process recognizes its own reply when fan-out reaches it.
	MemberID string
	Budgets  Budgets
}

// Unreachable is a discovered member that did not answer.
type Unreachable struct {
	Address string
	Message string
	// Code is a google.rpc.Code value.
	Code int32
}

// Result is one member's answer, or the failure to get one.
type Result[T any] struct {
	// ID is the member's self-reported identity.
	// It is empty when the member did not answer.
	ID string
	// Address is what was dialed.
	// It is empty for the local answer.
	Address string
	Value   T
	Err     error
}

// Roster records who was asked and who replied.
type Roster struct {
	Provider    string
	Discovered  int
	Responding  int
	Unreachable []Unreachable
}

// Handlers adapt one RPC to the plane.
// Self, Call and Merge are required.
type Handlers[Req, Resp any] struct {
	// Self answers for this process alone.
	Self func(ctx context.Context, req Req) Resp
	// Call invokes this same RPC on another proxy.
	Call func(ctx context.Context, cc grpc.ClientConnInterface, req Req) (Resp, error)
	// Merge folds the local answer and the members' answers into one.
	//
	// It takes the members as a slice rather than a pre-folded value.
	// An endpoint whose aggregate is not a sum can then still be expressed.
	// Shard ownership is disjoint across pods.
	// A configuration check wants to know whether the members agree, not what they add up to.
	//
	// It cannot fail.
	// Partial data is the answer to report, not an error.
	// A transport error would carry no roster and no partial results.
	// That is the opposite of what someone debugging a broken deployment needs.
	Merge func(self Result[Resp], members []Result[Resp], roster Roster) Resp
	// View narrows a response for the listener that will send it.
	// Serve always applies it last.
	// A handler cannot forget to.
	// Required for RoleCounterparty.
	View func(resp Resp, opts ServerOptions) Resp
	// ID reports which member produced a response.
	// Fan-out uses it to recognize this process's own reply and drop it.
	// Without it, a deployment that discovers itself counts itself twice.
	ID func(resp Resp) string
}

// Serve answers one RPC at whatever scope the caller asked for and the listener allows.
//
//	target set     forward once to that connection's peer proxy, for its deployment
//	member scope   this process only
//	group scope    this process plus every discovered sibling, each asked at member scope
//
// Siblings are asked at member scope and peer listeners refuse anything wider.
// A group call therefore produces exactly one round of fan-out.
func Serve[Req, Resp any](ctx context.Context, p *Plane, req Req, h Handlers[Req, Resp]) (Resp, error) {
	var zero Resp
	opts := OptionsFrom(ctx)

	if err := h.validate(opts); err != nil {
		return zero, err
	}

	if target := TargetFrom(ctx); target != "" {
		resp, err := forward(ctx, p, target, req, h)
		if err != nil {
			return zero, err
		}
		return h.view(resp, opts), nil
	}

	self := Result[Resp]{ID: p.MemberID, Value: h.Self(ctx, req)}

	if ScopeFrom(ctx) == ScopeMember {
		return h.view(self.Value, opts), nil
	}

	members, roster := fanOut(ctx, p, req, h)
	return h.view(h.Merge(self, members, roster), opts), nil
}

// validate refuses to answer rather than trusting that the handler was assembled correctly.
//
// The unset role matters as much as the missing View.
// Role is stamped by the interceptor.
// A listener that registers the service without installing it arrives here with the zero value.
// The zero value must not be the one that skips the narrowing.
func (h Handlers[Req, Resp]) validate(opts ServerOptions) error {
	switch {
	case h.Self == nil || h.Call == nil || h.Merge == nil:
		return status.Error(codes.Internal, "adminplane: endpoint is missing Self, Call or Merge")
	case opts.Role == RoleUnset:
		return status.Error(codes.Internal,
			"adminplane: listener did not install the adminplane interceptor, so no policy applies")
	case opts.Role == RoleCounterparty && h.View == nil:
		// A response reaching another organization must have been narrowed deliberately.
		// Narrowing by whichever fields the endpoint happened to populate is not deliberate.
		return status.Error(codes.Internal,
			"adminplane: endpoint has no View and may not be served to a peer cluster")
	}
	return nil
}

func (h Handlers[Req, Resp]) view(resp Resp, opts ServerOptions) Resp {
	if h.View == nil {
		return resp
	}
	return h.View(resp, opts)
}

// forward sends the call to the peer proxy of a named cluster connection.
// That proxy answers for its own deployment.
//
// Metadata is not inherited by outgoing calls.
// The forwarded request therefore carries only what is set here and cannot be forwarded again.
func forward[Req, Resp any](ctx context.Context, p *Plane, target string, req Req, h Handlers[Req, Resp]) (Resp, error) {
	var zero Resp
	if p.Peers == nil {
		return zero, status.Error(codes.Unimplemented, "this proxy cannot forward admin calls")
	}
	cc, err := p.Peers.PeerConn(target)
	if err != nil {
		return zero, err
	}
	budget := Clamp(ctx, p.Budgets.withDefaults().Forward)
	if budget <= 0 {
		return zero, status.Error(codes.DeadlineExceeded, "no time left to forward")
	}
	callCtx, cancel := context.WithTimeout(OutgoingGroupContext(ctx), budget)
	defer cancel()
	return h.Call(callCtx, cc, req)
}

// fanOut asks every discovered member for its own answer.
//
// Every discovered address is dialed, this process's own included.
// A DNS record carries no identity.
// Self cannot be recognized before the call.
// It is recognized afterwards by id and discarded then.
//
// Dialing this process's own address also means a peer listener that failed to bind shows up as unreachable.
// That is true and worth reporting.
func fanOut[Req, Resp any](ctx context.Context, p *Plane, req Req, h Handlers[Req, Resp]) ([]Result[Resp], Roster) {
	budgets := p.Budgets.withDefaults()
	// Serve has already taken this process's own answer by the time fanOut runs.
	// Every path out of here therefore has at least one responder.
	// The early returns below are failures to reach anyone else, not failures to answer.
	// Counting the local answer here rather than at the end makes all four of those paths right at once.
	roster := Roster{Provider: providerName(p.Discovery), Responding: 1}

	if p.Discovery == nil || p.Dial == nil {
		return nil, roster
	}

	discoverBudget := Clamp(ctx, budgets.Discover)
	if discoverBudget <= 0 {
		return nil, roster
	}
	discoverCtx, cancelDiscover := context.WithTimeout(ctx, discoverBudget)
	addresses, err := p.Discovery.Discover(discoverCtx)
	cancelDiscover()
	if err != nil {
		roster.Unreachable = append(roster.Unreachable, Unreachable{
			Message: "discovery failed: " + err.Error(),
			Code:    int32(codes.Unavailable),
		})
		return nil, roster
	}
	roster.Discovered = len(addresses)

	memberBudget := Clamp(ctx, budgets.Member)
	if memberBudget <= 0 {
		for _, address := range addresses {
			roster.Unreachable = append(roster.Unreachable, Unreachable{
				Address: address,
				Message: "no time left to query member",
				Code:    int32(codes.DeadlineExceeded),
			})
		}
		return nil, roster
	}

	results := make([]Result[Resp], len(addresses))
	sem := make(chan struct{}, maxConcurrentMembers)
	var wg sync.WaitGroup
	for i, address := range addresses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = callMember(ctx, p, address, memberBudget, req, h)
		}()
	}
	wg.Wait()

	var members []Result[Resp]
	for _, r := range results {
		if r.Err != nil {
			// This may be this process's own address.
			// Self is recognized by the id in a reply.
			// A failed dial has no reply.
			// A peer listener that did not bind therefore looks like any other unreachable member here.
			// Every sibling still reports it.
			// The bind failure itself is logged where it happens.
			roster.Unreachable = append(roster.Unreachable, Unreachable{
				Address: r.Address,
				Message: status.Convert(r.Err).Message(),
				Code:    int32(status.Code(r.Err)),
			})
			continue
		}
		if r.ID != "" && r.ID == p.MemberID {
			// This process's own reply, returned over its own peer listener.
			// The local answer already covers it.
			// Keeping both would double every count.
			continue
		}
		members = append(members, r)
	}
	roster.Responding += len(members)
	sort.Slice(roster.Unreachable, func(i, j int) bool {
		return roster.Unreachable[i].Address < roster.Unreachable[j].Address
	})
	sort.SliceStable(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members, roster
}

func callMember[Req, Resp any](
	ctx context.Context, p *Plane, address string, budget time.Duration, req Req, h Handlers[Req, Resp],
) Result[Resp] {
	result := Result[Resp]{Address: address}
	callCtx, cancel := context.WithTimeout(OutgoingMemberContext(ctx), budget)
	defer cancel()

	cc, release, err := p.Dial(callCtx, address)
	if err != nil {
		result.Err = err
		return result
	}
	defer release()

	value, err := h.Call(callCtx, cc, req)
	if err != nil {
		result.Err = err
		return result
	}
	result.Value = value
	if h.ID != nil {
		result.ID = h.ID(value)
	}
	return result
}

func providerName(d Discovery) string {
	if d == nil {
		return ""
	}
	return d.Provider()
}
