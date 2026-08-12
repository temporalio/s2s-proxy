package proxy

import (
	"context"
	"sort"
	"strings"

	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/adminplane"
	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
)

// adminSelfDescriber produces this process's own view of its cluster connections.
type adminSelfDescriber interface {
	SelfClusterConnections(ctx context.Context) *proxyadminv1.DescribeClusterConnectionsResponse
}

type proxyAdminServiceServer struct {
	proxyadminv1.UnimplementedProxyAdminServiceServer
	self  adminSelfDescriber
	plane *adminplane.Plane
}

// NewProxyAdminServiceServer builds the admin service.
// One instance serves every listener.
// What differs between them is the ServerOptions their interceptor applies.
func NewProxyAdminServiceServer(self adminSelfDescriber, plane *adminplane.Plane) proxyadminv1.ProxyAdminServiceServer {
	return &proxyAdminServiceServer{self: self, plane: plane}
}

type (
	describeRequest  = *proxyadminv1.DescribeClusterConnectionsRequest
	describeResponse = *proxyadminv1.DescribeClusterConnectionsResponse
)

func (s *proxyAdminServiceServer) DescribeClusterConnections(
	ctx context.Context, req describeRequest,
) (describeResponse, error) {
	return adminplane.Serve(ctx, s.plane, req, adminplane.Handlers[describeRequest, describeResponse]{
		Self: func(ctx context.Context, _ describeRequest) describeResponse {
			return s.self.SelfClusterConnections(ctx)
		},
		Call: func(ctx context.Context, cc grpc.ClientConnInterface, req describeRequest) (describeResponse, error) {
			return proxyadminv1.NewProxyAdminServiceClient(cc).DescribeClusterConnections(ctx, req)
		},
		Merge: mergeDescribeClusterConnections,
		View:  viewDescribeClusterConnections,
		ID:    func(resp describeResponse) string { return selfIdentity(resp).GetId() },
	})
}

// viewDescribeClusterConnections narrows a response to what the receiving listener may see.
//
// A counterparty is another organization reached over a mux.
// It gets the state of the link it holds the other end of.
// Nothing else.
//
// The response is built field by field rather than by deleting fields.
// A field added to the proto later is not served across an organizational boundary until someone adds it here.
//
// Two things are withheld deliberately:
//
//   - This deployment's shape. The per-pod rows in Members carry pod ids, addresses and versions,
//     and how many rows there are is this deployment's replica count.
//   - Every other cluster connection. Each belongs to a different migration.
func viewDescribeClusterConnections(resp describeResponse, opts adminplane.ServerOptions) describeResponse {
	if opts.Role != adminplane.RoleCounterparty || resp == nil {
		return resp
	}
	out := &proxyadminv1.DescribeClusterConnectionsResponse{}
	for _, cc := range resp.GetClusterConnections() {
		if cc.GetName() != opts.ConnectionName {
			continue
		}
		out.ClusterConnections = append(out.ClusterConnections, &proxyadminv1.ClusterConnection{
			Name:                 cc.GetName(),
			State:                cc.GetState(),
			MuxSessionsConnected: cc.GetMuxSessionsConnected(),
			MuxSessionsTotal:     cc.GetMuxSessionsTotal(),
			MuxSessionsTarget:    cc.GetMuxSessionsTarget(),
		})
	}
	return out
}

// mergeDescribeClusterConnections folds the members' answers into one deployment-wide answer.
func mergeDescribeClusterConnections(
	self adminplane.Result[describeResponse],
	members []adminplane.Result[describeResponse],
	roster adminplane.Roster,
) describeResponse {
	all := make([]adminplane.Result[describeResponse], 0, len(members)+1)
	all = append(all, self)
	all = append(all, members...)

	out := &proxyadminv1.DescribeClusterConnectionsResponse{}

	// Iterate the union of names, not just this member's.
	// A member missing a connection another member has is the half-applied configuration this endpoint exists to surface.
	// Intersecting would hide it.
	for _, name := range unionOfConnectionNames(all) {
		out.ClusterConnections = append(out.ClusterConnections, mergeConnection(name, all, roster))
	}
	return out
}

func unionOfConnectionNames(all []adminplane.Result[describeResponse]) []string {
	seen := map[string]struct{}{}
	var names []string
	for _, r := range all {
		for _, cc := range r.Value.GetClusterConnections() {
			if _, ok := seen[cc.GetName()]; ok {
				continue
			}
			seen[cc.GetName()] = struct{}{}
			names = append(names, cc.GetName())
		}
	}
	sort.Strings(names)
	return names
}

func mergeConnection(
	name string, all []adminplane.Result[describeResponse], roster adminplane.Roster,
) *proxyadminv1.ClusterConnection {
	merged := &proxyadminv1.ClusterConnection{Name: name}

	var states []proxyadminv1.ConnectionState

	// all[0] is this process's own answer.
	// Every other element came from a member this process dialed.
	for i, r := range all {
		src := memberEntry(name, r)

		// Built field by field rather than mutated.
		// src points into the member's own answer.
		// Later connections read that answer again.
		//
		// The reporting pod set self on its own row.
		// Only the aggregator knows which pod served this response.
		// Only the aggregator knows what address each member was reached at.
		srcID := src.GetIdentity()
		entry := &proxyadminv1.ClusterConnectionMember{
			Identity: &proxyadminv1.Member{
				Id:        srcID.GetId(),
				Self:      i == 0,
				Address:   r.Address,
				Version:   srcID.GetVersion(),
				StartTime: srcID.GetStartTime(),
			},
			State:                src.GetState(),
			MuxSessionsConnected: src.GetMuxSessionsConnected(),
			MuxSessionsTotal:     src.GetMuxSessionsTotal(),
			MuxSessionsTarget:    src.GetMuxSessionsTarget(),
		}

		merged.Members = append(merged.Members, entry)
		states = append(states, entry.GetState())

		merged.MuxSessionsConnected += entry.GetMuxSessionsConnected()
		merged.MuxSessionsTotal += entry.GetMuxSessionsTotal()
		merged.MuxSessionsTarget += entry.GetMuxSessionsTarget()
	}

	sortMembers(merged.Members)
	merged.State = rollup(states, len(roster.Unreachable) > 0)
	return merged
}

// selfIdentity returns the identity a response's own pod reported for itself.
func selfIdentity(resp describeResponse) *proxyadminv1.Member {
	for _, cc := range resp.GetClusterConnections() {
		for _, m := range cc.GetMembers() {
			if m.GetIdentity().GetSelf() {
				return m.GetIdentity()
			}
		}
	}
	return nil
}

// memberEntry finds one member's entry for a connection.
// It synthesizes an errored entry when that member does not have the connection at all.
func memberEntry(name string, r adminplane.Result[describeResponse]) *proxyadminv1.ClusterConnectionMember {
	for _, cc := range r.Value.GetClusterConnections() {
		if cc.GetName() != name {
			continue
		}
		// A member-scoped response carries exactly one entry: the responder itself.
		if entries := cc.GetMembers(); len(entries) > 0 {
			return entries[0]
		}
	}
	// The member answered, it just has no connection by this name.
	// Its identity is carried across from whichever connection it did report.
	// The row still names the pod that disagrees.
	//
	// MuxSessionsTarget stays zero.
	// That is what separates this row from a member that has the connection and cannot hold it.
	identity := selfIdentity(r.Value)
	return &proxyadminv1.ClusterConnectionMember{
		Identity: &proxyadminv1.Member{
			Id:      identity.GetId(),
			Version: identity.GetVersion(),
		},
		State: proxyadminv1.ConnectionState_CONNECTION_STATE_ERROR,
	}
}

// rollup reduces member states to one.
//
// Anything other than every member reporting CONNECTED is ERROR.
//
// Three conditions are what aggregating exists to surface:
//
//   - a member that is degraded
//   - a member that does not have this connection at all
//   - a member that never answered
//
// None of them can be rolled up to anything healthier.
//
// anyUnreachable is read separately because a member that never replied contributes no state at all.
// Without it a deployment with a dead pod would report exactly the same as a healthy one.
func rollup(states []proxyadminv1.ConnectionState, anyUnreachable bool) proxyadminv1.ConnectionState {
	const (
		unspecified = proxyadminv1.ConnectionState_CONNECTION_STATE_UNSPECIFIED
		connected   = proxyadminv1.ConnectionState_CONNECTION_STATE_CONNECTED
		errored     = proxyadminv1.ConnectionState_CONNECTION_STATE_ERROR
	)

	var counted int
	for _, s := range states {
		if s == unspecified {
			// A connection that is not multiplexed.
			// It has no session state to contribute.
			continue
		}
		counted++
		if s != connected {
			return errored
		}
	}

	switch {
	case counted == 0:
		return unspecified
	case anyUnreachable:
		return errored
	default:
		return connected
	}
}

// sortMembers orders members by id so repeated calls return the same list.
// Members that did not report an id sort last.
func sortMembers(members []*proxyadminv1.ClusterConnectionMember) {
	sort.SliceStable(members, func(i, j int) bool {
		a, b := members[i].GetIdentity().GetId(), members[j].GetIdentity().GetId()
		if (a == "") != (b == "") {
			return b == ""
		}
		return strings.Compare(a, b) < 0
	})
}
