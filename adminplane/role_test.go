package adminplane

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
)

const (
	describeRPC = proxyadminv1.ProxyAdminService_DescribeClusterConnections_FullMethodName
	// futureRPC stands for the next RPC someone generates.
	// It is spelled as a literal because it does not exist yet.
	// The ceiling must refuse it until somebody lists it.
	futureRPC       = "/temporal.s2sproxy.proxyadmin.v1.ProxyAdminService/MutateSomething"
	replicationRPC  = "/temporal.server.api.adminservice.v1.AdminService/StreamWorkflowReplicationMessages"
	otherServiceRPC = "/temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution"
)

// invoke runs the unary interceptor and reports what the handler saw.
func invoke(t *testing.T, o ServerOptions, method string, md ...string) (context.Context, error) {
	t.Helper()
	ctx := context.Background()
	if len(md) > 0 {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(md...))
	}
	var seen context.Context
	_, err := UnaryInterceptor(o)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(ctx context.Context, _ any) (any, error) {
			seen = ctx
			return nil, nil
		})
	return seen, err
}

func TestRoleUnsetIsRejected(t *testing.T) {
	// A registration that forgets to state its role must fail rather than default to the most permissive option.
	_, err := invoke(t, ServerOptions{}, describeRPC)
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestCounterpartyRequiresAConnectionName(t *testing.T) {
	// Without a name there is nothing to narrow the answer to.
	_, err := invoke(t, ServerOptions{Role: RoleCounterparty}, describeRPC)
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestScopeResolution(t *testing.T) {
	for _, tc := range []struct {
		name      string
		opts      ServerOptions
		scopeMD   string
		wantScope Scope
		wantCode  codes.Code
	}{
		{name: "operator defaults to group", opts: ServerOptions{Role: RoleOperator}, wantScope: ScopeGroup},
		{name: "operator may ask for member", opts: ServerOptions{Role: RoleOperator}, scopeMD: "member", wantScope: ScopeMember},
		{name: "operator may ask for group", opts: ServerOptions{Role: RoleOperator}, scopeMD: "group", wantScope: ScopeGroup},
		// A header-less call to the peer listener still works.
		// It gets a member answer.
		{name: "peer defaults to member", opts: ServerOptions{Role: RolePeer}, wantScope: ScopeMember},
		// Refused rather than narrowed.
		// A member answer and a group answer have the same shape.
		// A caller could not tell it had been downgraded.
		{name: "peer refuses group", opts: ServerOptions{Role: RolePeer}, scopeMD: "group", wantCode: codes.PermissionDenied},
		{name: "counterparty defaults to group", opts: ServerOptions{Role: RoleCounterparty, ConnectionName: "c"}, wantScope: ScopeGroup},
		// Named so a newer caller fails loudly instead of silently receiving less than it asked for.
		{name: "topology is not implemented", opts: ServerOptions{Role: RoleOperator}, scopeMD: "topology", wantCode: codes.Unimplemented},
		{name: "unknown scope is rejected", opts: ServerOptions{Role: RoleOperator}, scopeMD: "galaxy", wantCode: codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var md []string
			if tc.scopeMD != "" {
				md = []string{MDScope, tc.scopeMD}
			}
			ctx, err := invoke(t, tc.opts, describeRPC, md...)
			if tc.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantScope, ScopeFrom(ctx))
			require.Equal(t, tc.opts, OptionsFrom(ctx))
		})
	}
}

func TestForwardingIsOperatorOnly(t *testing.T) {
	for _, role := range []ServerOptions{
		{Role: RolePeer},
		{Role: RoleCounterparty, ConnectionName: "c"},
	} {
		t.Run(role.Role.String(), func(t *testing.T) {
			// Otherwise a peer could pivot through this proxy into a third cluster.
			_, err := invoke(t, role, describeRPC, MDTarget, "somewhere")
			require.Error(t, err)
			require.Equal(t, codes.PermissionDenied, status.Code(err))
		})
	}

	ctx, err := invoke(t, ServerOptions{Role: RoleOperator}, describeRPC, MDTarget, "somewhere")
	require.NoError(t, err)
	require.Equal(t, "somewhere", TargetFrom(ctx))
}

// The allowlist stops an RPC added later from being served across an organizational boundary without anyone deciding that it should be.
func TestCounterpartyMethodAllowlist(t *testing.T) {
	counterparty := ServerOptions{Role: RoleCounterparty, ConnectionName: "c"}

	_, err := invoke(t, counterparty, describeRPC)
	require.NoError(t, err)

	_, err = invoke(t, counterparty, futureRPC)
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))

	// The operator and peer listeners serve the whole admin service.
	_, err = invoke(t, ServerOptions{Role: RoleOperator}, futureRPC)
	require.NoError(t, err)
}

// The mux server carries replication traffic through the same interceptor chain.
// One yamux session serves one gRPC server.
// grpc-go has no per-service interceptor.
// Gating anything outside the admin service would stop replication.
func TestNonAdminMethodsPassThroughUntouched(t *testing.T) {
	counterparty := ServerOptions{Role: RoleCounterparty, ConnectionName: "c"}

	for _, method := range []string{replicationRPC, otherServiceRPC} {
		t.Run(method, func(t *testing.T) {
			ctx, err := invoke(t, counterparty, method)
			require.NoError(t, err)
			require.Equal(t, ServerOptions{}, OptionsFrom(ctx),
				"a method outside the admin service must not be stamped with listener policy")
		})
	}

	// An unset role is otherwise rejected.
	// Even so, it must not block replication.
	_, err := invoke(t, ServerOptions{}, replicationRPC)
	require.NoError(t, err)
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

// Unary-only enforcement would fail open for the first streaming admin RPC anyone adds.
func TestStreamInterceptorEnforcesTheSamePolicy(t *testing.T) {
	counterparty := ServerOptions{Role: RoleCounterparty, ConnectionName: "c"}

	err := StreamInterceptor(counterparty)(nil, &fakeStream{ctx: context.Background()},
		&grpc.StreamServerInfo{FullMethod: futureRPC},
		func(any, grpc.ServerStream) error { return nil })
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))

	var seen context.Context
	err = StreamInterceptor(counterparty)(nil, &fakeStream{ctx: context.Background()},
		&grpc.StreamServerInfo{FullMethod: describeRPC},
		func(_ any, ss grpc.ServerStream) error {
			seen = ss.Context()
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, ScopeGroup, ScopeFrom(seen))
}

// The obvious formulation, min(want, time.Until(deadline)-slack), goes hugely negative for a context with no deadline.
// That is the context grpcurl produces.
func TestClampWithoutADeadlineKeepsTheFullBudget(t *testing.T) {
	require.Equal(t, 2*time.Second, Clamp(context.Background(), 2*time.Second))
}

func TestClampShortensToFitADeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	got := Clamp(ctx, 2*time.Second)
	require.Less(t, got, 500*time.Millisecond)
	require.Positive(t, got)
}

// Egress is a separate decision from ingress.
// The far side may well implement a method.
// This is the local choice not to send it.
func TestForwardingIsRefusedForAMethodOutsideTheCeiling(t *testing.T) {
	_, err := invoke(t, ServerOptions{Role: RoleOperator}, futureRPC, MDTarget, "cluster-b")
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "not forwarded")

	// Without a target the same method is served locally.
	// The refusal is about crossing the boundary rather than about the method existing.
	_, err = invoke(t, ServerOptions{Role: RoleOperator}, futureRPC)
	require.NoError(t, err)
}

// Configuration narrows the ceiling.
// It can never widen it.
func TestOperatorNarrowingOfTheCounterparty(t *testing.T) {
	counterparty := func(methods []string) ServerOptions {
		return ServerOptions{Role: RoleCounterparty, ConnectionName: "cluster-b", Methods: methods}
	}
	cases := []struct {
		name    string
		methods []string
		method  string
		wantErr codes.Code
	}{
		{name: "absent means the ceiling applies", methods: nil, method: describeRPC},
		{name: "absent still refuses a method outside the ceiling", methods: nil, method: futureRPC, wantErr: codes.Unimplemented},
		{name: "listing the method serves it", methods: []string{describeRPC}, method: describeRPC},
		// This is how an operator declines to answer one counterparty at all.
		// Methods is a slice rather than a set type so that absent and empty stay distinct.
		// auth.AccessControl cannot express the distinction: its empty list means allow everything.
		{name: "an empty list serves nothing", methods: []string{}, method: describeRPC, wantErr: codes.Unimplemented},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := invoke(t, counterparty(c.methods), c.method)
			if c.wantErr == codes.OK {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, c.wantErr, status.Code(err))
		})
	}

	// Narrowing applies only where it is configured.
	// The operator listener is inside the trust boundary and is unaffected.
	_, err := invoke(t, ServerOptions{Role: RoleOperator}, describeRPC)
	require.NoError(t, err)
}

func TestResolveCounterpartyMethods(t *testing.T) {
	// Absent and empty mean opposite things once they reach ServerOptions.
	// They must stay distinguishable all the way through.
	resolved, err := ResolveCounterpartyMethods(nil)
	require.NoError(t, err)
	require.Nil(t, resolved)

	resolved, err = ResolveCounterpartyMethods([]string{})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Empty(t, resolved)

	resolved, err = ResolveCounterpartyMethods([]string{"DescribeClusterConnections"})
	require.NoError(t, err)
	require.Equal(t, []string{describeRPC}, resolved)

	// A typo is a startup error rather than a method that silently stops being served.
	_, err = ResolveCounterpartyMethods([]string{"DescribeClusterConnection"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DescribeClusterConnections")
}

// Clamp returning non-positive makes forward and fanOut report a deadline rather than dialing with no time left.
// Without a case here neither path is ever exercised.
func TestClampGoesNonPositiveOnceTheDeadlineHasPassed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()
	require.Negative(t, Clamp(ctx, 2*time.Second))
}
