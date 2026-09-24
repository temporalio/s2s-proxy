package adminplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
)

const (
	describeRPC     = proxyadminv1.ProxyAdminService_DescribeClusterConnections_FullMethodName
	futureRPC       = "/temporal.s2sproxy.proxyadmin.v1.ProxyAdminService/MutateSomething"
	replicationRPC  = "/temporal.server.api.adminservice.v1.AdminService/StreamWorkflowReplicationMessages"
	otherServiceRPC = "/temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution"
)

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
	_, err := invoke(t, ServerOptions{}, describeRPC)
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestCounterpartyRequiresAConnectionName(t *testing.T) {
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
		{name: "peer defaults to member", opts: ServerOptions{Role: RolePeer}, wantScope: ScopeMember},
		{name: "peer refuses group", opts: ServerOptions{Role: RolePeer}, scopeMD: "group", wantCode: codes.PermissionDenied},
		{name: "counterparty defaults to group", opts: ServerOptions{Role: RoleCounterparty, ConnectionName: "c"}, wantScope: ScopeGroup},
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
			_, err := invoke(t, role, describeRPC, MDTarget, "somewhere")
			require.Error(t, err)
			require.Equal(t, codes.PermissionDenied, status.Code(err))
		})
	}

	ctx, err := invoke(t, ServerOptions{Role: RoleOperator}, describeRPC, MDTarget, "somewhere")
	require.NoError(t, err)
	require.Equal(t, "somewhere", TargetFrom(ctx))
}

func TestCounterpartyMethodAllowlist(t *testing.T) {
	counterparty := ServerOptions{Role: RoleCounterparty, ConnectionName: "c"}

	_, err := invoke(t, counterparty, describeRPC)
	require.NoError(t, err)

	_, err = invoke(t, counterparty, futureRPC)
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = invoke(t, ServerOptions{Role: RoleOperator}, futureRPC)
	require.NoError(t, err)
}

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

	_, err := invoke(t, ServerOptions{}, replicationRPC)
	require.NoError(t, err)
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

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

func TestForwardingIsRefusedForAMethodOutsideTheCeiling(t *testing.T) {
	_, err := invoke(t, ServerOptions{Role: RoleOperator}, futureRPC, MDTarget, "cluster-b")
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "not forwarded")

	_, err = invoke(t, ServerOptions{Role: RoleOperator}, futureRPC)
	require.NoError(t, err)
}

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

	_, err := invoke(t, ServerOptions{Role: RoleOperator}, describeRPC)
	require.NoError(t, err)
}

func TestResolveCounterpartyMethods(t *testing.T) {
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

	_, err = ResolveCounterpartyMethods([]string{"DescribeClusterConnection"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DescribeClusterConnections")
}
