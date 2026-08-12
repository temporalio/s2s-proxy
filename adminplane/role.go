package adminplane

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
)

// Metadata keys carrying the routing decisions for an admin call.
//
// They are metadata rather than request fields because an interceptor cannot read a request field without reflection.
// With request fields, every listener's limits have to be re-checked inside every handler.
// An RPC added later is then exposed until someone remembers to check it.
// With metadata, one interceptor covers every RPC.
const (
	// MDScope is "member" or "group". Absent means group.
	MDScope = "s2s-proxy-scope"
	// MDTarget names a cluster connection whose peer proxy should answer instead.
	MDTarget = "s2s-proxy-target"
)

// Scope is how far a call travels. Values are ordered by increasing breadth.
type Scope int

const (
	// ScopeMember answers for the receiving process only.
	ScopeMember Scope = iota
	// ScopeGroup aggregates across the pods of the receiving proxy deployment.
	ScopeGroup
)

const (
	scopeMemberValue = "member"
	scopeGroupValue  = "group"
	// scopeTopologyValue is reserved for a scope that also covers the far side of every cluster connection.
	// Recognizing it makes asking for it fail loudly instead of returning a narrower result that looks the same.
	scopeTopologyValue = "topology"
)

// Role is a listener's position in the topology.
// It determines how far calls arriving there may travel and how much of the answer they see.
type Role int

const (
	// RoleUnset is the zero value and is rejected.
	// A registration that forgets to state its role fails at startup rather than defaulting to the most permissive option.
	RoleUnset Role = iota
	// RoleOperator is a trusted local listener.
	// Any scope, forwarding allowed, nothing withheld.
	RoleOperator
	// RolePeer is reached by the other pods of this same deployment.
	// They only ever need this process's own answer.
	// Member scope is forced and forwarding is refused.
	// A group call therefore makes a single round of fan-out.
	RolePeer
	// RoleCounterparty is reached over a mux by a proxy belonging to a different organization.
	// Answers are narrowed to the cluster connection the call arrived on.
	// Only explicitly listed methods are served.
	RoleCounterparty
)

func (r Role) String() string {
	switch r {
	case RoleOperator:
		return "operator"
	case RolePeer:
		return "peer"
	case RoleCounterparty:
		return "counterparty"
	}
	return "unset"
}

// ServerOptions is the policy for one listener.
type ServerOptions struct {
	Role Role
	// ConnectionName is the cluster connection a RoleCounterparty listener belongs to.
	// Answers are narrowed to it.
	ConnectionName string
	// Methods narrows what this listener serves, below the compile-time ceiling in adminMethods.
	// Full method names, resolved by ResolveCounterpartyMethods.
	//
	// Nil means no narrowing.
	// A present-but-empty slice means serve nothing.
	// An operator switches the admin plane off for one counterparty by writing the empty slice.
	//
	// auth.AccessControl cannot express that: its IsAllowed returns true for an empty list.
	Methods []string
}

// serves reports whether the operator's configuration permits this method here.
// Configuration can only narrow.
// The compile-time ceiling is checked separately.
func (o ServerOptions) serves(fullMethod string) bool {
	if o.Methods == nil {
		return true
	}
	return slices.Contains(o.Methods, fullMethod)
}

// Validate rejects a policy that cannot be enforced.
func (o ServerOptions) Validate() error {
	switch o.Role {
	case RoleUnset:
		return fmt.Errorf("adminplane: ServerOptions.Role must be set")
	case RoleCounterparty:
		if o.ConnectionName == "" {
			// Without a name there is nothing to narrow to.
			// The listener would answer for every cluster connection on this proxy.
			// That includes connections belonging to other organizations.
			return fmt.Errorf("adminplane: RoleCounterparty requires ConnectionName")
		}
	}
	return nil
}

type optionsKey struct{}

// OptionsFrom returns the policy of the listener that accepted this call.
func OptionsFrom(ctx context.Context) ServerOptions {
	o, _ := ctx.Value(optionsKey{}).(ServerOptions)
	return o
}

type scopeKey struct{}

// ScopeFrom returns the resolved scope for this call.
// The interceptor sets it from the listener's limits.
func ScopeFrom(ctx context.Context) Scope {
	s, _ := ctx.Value(scopeKey{}).(Scope)
	return s
}

// TargetFrom returns the cluster connection this call should be forwarded to, or "".
func TargetFrom(ctx context.Context) string {
	return firstMD(ctx, MDTarget)
}

func firstMD(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}

// OutgoingMemberContext marks an outgoing call as member-scoped, for fanning out to siblings.
func OutgoingMemberContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, MDScope, scopeMemberValue)
}

// OutgoingGroupContext marks an outgoing call as group-scoped.
// The far proxy aggregates its own deployment.
func OutgoingGroupContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, MDScope, scopeGroupValue)
}

// adminServicePrefix is the gRPC method prefix this package governs.
// Deriving it from the generated descriptor rather than injecting it means it cannot be empty.
//
// Methods outside it pass through untouched.
// One yamux session serves one grpc.Server.
// On a mux this interceptor is therefore installed server-wide and also sees every replication call.
// A mistake in the admin policy must not be able to stop the data path.
var adminServicePrefix = "/" + proxyadminv1.ProxyAdminService_ServiceDesc.ServiceName + "/"

// methodPolicy is what a single admin RPC is allowed to do.
// Both fields default to false.
// An RPC that nobody has thought about is neither served to another organization nor sent to one.
type methodPolicy struct {
	// Counterparty allows another organization to call this method on us over a mux.
	Counterparty bool
	// Forwardable allows this proxy to send this method to another organization, via s2s-proxy-target.
	Forwardable bool
}

// adminMethods is the capability ceiling.
// It is keyed by the generated method constants.
// A rename is therefore a compile error.
//
// The two columns are separate trust decisions.
// What another organization may ask this proxy is not the same question as what this proxy may ask them.
// Adding an RPC forces an explicit answer to both.
//
// There is no entry for the operator or peer listeners.
// Both sit inside this deployment's trust boundary and serve whatever the process registered.
// A hand-maintained list for them would misfire the first time someone added an RPC and forgot it.
// The symptom would be Unimplemented on the operator listener.
// That is the listener an operator debugs from.
// The peer listener also has to serve whatever the operator fans out, or group scope breaks.
var adminMethods = map[string]methodPolicy{
	proxyadminv1.ProxyAdminService_DescribeClusterConnections_FullMethodName: {
		Counterparty: true,
		Forwardable:  true,
	},
}

// authorize resolves the scope for a call and rejects anything the listener does not permit.
func authorize(ctx context.Context, o ServerOptions, fullMethod string) (context.Context, error) {
	// Namespace check first, before anything that can fail. See adminServicePrefix.
	if !strings.HasPrefix(fullMethod, adminServicePrefix) {
		return ctx, nil
	}

	if err := o.Validate(); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	policy := adminMethods[fullMethod]

	if o.Role == RoleCounterparty {
		// The ceiling first, then whatever narrowing the operator configured on top of it.
		if !policy.Counterparty || !o.serves(fullMethod) {
			return nil, status.Errorf(codes.Unimplemented,
				"method %s is not served to a peer cluster", fullMethod)
		}
	}

	if target := TargetFrom(ctx); target != "" {
		if o.Role != RoleOperator {
			return nil, status.Errorf(codes.PermissionDenied,
				"%s may not be set on a %s listener", MDTarget, o.Role)
		}
		// Fail closed on egress.
		// Whether this proxy may put a question to another organization is a separate decision from whether that organization would answer it.
		// It is also the only half of the exchange this proxy controls.
		if !policy.Forwardable {
			return nil, status.Errorf(codes.PermissionDenied,
				"method %s is not forwarded to a peer cluster", fullMethod)
		}
	}

	scope, err := resolveScope(ctx, o)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, optionsKey{}, o)
	ctx = context.WithValue(ctx, scopeKey{}, scope)
	return ctx, nil
}

func resolveScope(ctx context.Context, o ServerOptions) (Scope, error) {
	requested := firstMD(ctx, MDScope)
	switch requested {
	case "":
		// Absent means group, clamped by the listener.
		// A header-less call to a peer listener gets a member answer rather than an error.
		// Plain grpcurl therefore still works there.
		if o.Role == RolePeer {
			return ScopeMember, nil
		}
		return ScopeGroup, nil
	case scopeMemberValue:
		return ScopeMember, nil
	case scopeGroupValue:
		if o.Role == RolePeer {
			// Refuse rather than silently narrowing.
			// A member answer and a group answer have the same shape.
			// A caller could not tell it had been downgraded.
			return 0, status.Errorf(codes.PermissionDenied,
				"scope %q is not served on a %s listener", requested, o.Role)
		}
		return ScopeGroup, nil
	case scopeTopologyValue:
		return 0, status.Errorf(codes.Unimplemented, "scope %q is not implemented", requested)
	default:
		return 0, status.Errorf(codes.InvalidArgument,
			"unknown %s %q, want %q or %q", MDScope, requested, scopeMemberValue, scopeGroupValue)
	}
}

// UnaryInterceptor enforces o on unary calls.
func UnaryInterceptor(o ServerOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := authorize(ctx, o, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamInterceptor enforces o on streaming calls.
// Without it, the first streaming admin RPC would bypass every check above.
func StreamInterceptor(o ServerOptions) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := authorize(ss.Context(), o, info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// ResolveCounterpartyMethods maps the short method names an operator writes in configuration onto the full names the interceptor matches.
// A name a counterparty could never be served is rejected.
//
// Configuration can only narrow the compile-time ceiling.
// A name outside it is a mistake rather than a wider grant.
// It fails at startup instead of silently denying at request time.
// Short names are what the sibling aclPolicy.allowedMethods.adminService list already uses.
//
// A nil input returns nil: no narrowing.
// An empty non-nil input returns an empty non-nil slice: serve nothing.
func ResolveCounterpartyMethods(shortNames []string) ([]string, error) {
	if shortNames == nil {
		return nil, nil
	}
	resolved := make([]string, 0, len(shortNames))
	for _, short := range shortNames {
		full, err := counterpartyMethodByShortName(short)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, full)
	}
	return resolved, nil
}

func counterpartyMethodByShortName(short string) (string, error) {
	var known []string
	for full, policy := range adminMethods {
		if !policy.Counterparty {
			continue
		}
		name := full[strings.LastIndex(full, "/")+1:]
		if name == short {
			return full, nil
		}
		known = append(known, name)
	}
	slices.Sort(known)
	return "", fmt.Errorf("method %q cannot be served to a peer cluster, want one of %v", short, known)
}
