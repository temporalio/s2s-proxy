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

// Metadata keys that route an admin call.
const (
	// MDScope is "member" or "group".
	// Absent means group.
	MDScope = "s2s-proxy-scope"
	// MDTarget names a cluster connection whose peer proxy should answer instead.
	MDTarget = "s2s-proxy-target"
)

// Scope is how far a call travels.
// Values are ordered by increasing breadth.
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
	// scopeTopologyValue is recognized but not implemented.
	scopeTopologyValue = "topology"
)

// Role is a listener's position in the topology.
// It determines how far calls arriving there may travel and how much of the answer they see.
type Role int

const (
	// RoleUnset is the zero value and is rejected.
	RoleUnset Role = iota
	// RoleOperator is a trusted local listener.
	// Any scope, forwarding allowed, nothing withheld.
	RoleOperator
	// RolePeer is reached by the other pods of this same deployment.
	// Member scope is forced and forwarding is refused.
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
	// Methods narrows what this listener serves, below the ceiling in adminMethods.
	// ResolveCounterpartyMethods produces the full method names it holds.
	//
	// Nil means no narrowing.
	// An empty non-nil slice serves nothing.
	Methods []string
}

// serves reports whether the operator's configuration permits this method here.
// The ceiling in adminMethods is checked separately.
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

// OutgoingMemberContext marks an outgoing call as member-scoped.
func OutgoingMemberContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, MDScope, scopeMemberValue)
}

// OutgoingGroupContext marks an outgoing call as group-scoped.
func OutgoingGroupContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, MDScope, scopeGroupValue)
}

// adminServicePrefix is the gRPC method prefix this package governs.
// Methods outside it pass through untouched.
var adminServicePrefix = "/" + proxyadminv1.ProxyAdminService_ServiceDesc.ServiceName + "/"

// methodPolicy is what a single admin RPC is allowed to do.
// Both fields default to false.
type methodPolicy struct {
	// Counterparty allows another organization to call this method on this proxy.
	Counterparty bool
	// Forwardable allows this proxy to send this method to another organization.
	Forwardable bool
}

// adminMethods is the capability ceiling.
// Its keys are the generated method constants.
// The two columns are independent.
//
// The operator and peer listeners have no entry and serve whatever the process registered.
var adminMethods = map[string]methodPolicy{
	proxyadminv1.ProxyAdminService_DescribeClusterConnections_FullMethodName: {
		Counterparty: true,
		Forwardable:  true,
	},
}

// authorize resolves the scope for a call and rejects anything the listener does not permit.
func authorize(ctx context.Context, o ServerOptions, fullMethod string) (context.Context, error) {
	if !strings.HasPrefix(fullMethod, adminServicePrefix) {
		return ctx, nil
	}

	if err := o.Validate(); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	policy := adminMethods[fullMethod]

	if o.Role == RoleCounterparty {
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
		// Absent means group.
		// The listener clamps it.
		if o.Role == RolePeer {
			return ScopeMember, nil
		}
		return ScopeGroup, nil
	case scopeMemberValue:
		return ScopeMember, nil
	case scopeGroupValue:
		if o.Role == RolePeer {
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

// ResolveCounterpartyMethods maps short method names from configuration onto full method names.
// The interceptor matches the full names.
// A name outside the ceiling in adminMethods is an error.
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
