package adminplane

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"

	"github.com/temporalio/s2s-proxy/config"
)

// maxDiscoveredMembers bounds a single discovery result.
// A name pointing at the wrong service then fans out to a bounded number of addresses rather than the whole cluster.
const maxDiscoveredMembers = 256

// Discovery enumerates the peer admin addresses of the other pods in this proxy deployment.
//
// It reports addresses, not identities.
// A DNS A record carries no name.
// A member is only identified once it answers.
// Callers recognize themselves by comparing the id in a response, never by address.
type Discovery interface {
	// Provider names the mechanism, for inclusion in responses.
	// Without it a roster of one is ambiguous between a single-replica deployment and a deployment with discovery switched off.
	Provider() string
	Discover(ctx context.Context) ([]string, error)
}

// NewDiscovery builds the provider named in cfg.
// defaultPort is the port the peer listener binds, for providers that resolve names without ports.
//
// The configuration is assumed to have been validated already.
// config.DiscoveryConfig.Validate owns the rules for what a usable provider block looks like.
// Duplicating them here produced two ceilings that had already drifted apart.
// The unknown-provider case remains because this is also reachable from a caller that built the config in Go.
//
// Adding a provider means a case here and a constructor below.
func NewDiscovery(cfg config.DiscoveryConfig, defaultPort int) (Discovery, error) {
	switch cfg.Provider {
	case "", config.DiscoveryNone:
		return NoDiscovery(), nil
	case config.DiscoveryDNS:
		port := cfg.DNS.Port
		if port == 0 {
			port = defaultPort
		}
		return NewDNSDiscovery(cfg.DNS.Name, port), nil
	case config.DiscoveryStatic:
		return NewStaticDiscovery(cfg.Static.Addresses), nil
	default:
		return nil, fmt.Errorf("unknown discovery provider %q, want one of %v",
			cfg.Provider, config.DiscoveryProviders)
	}
}

type noDiscovery struct{}

// NoDiscovery reports no peers.
// A proxy configured with it only ever describes itself.
func NoDiscovery() Discovery { return noDiscovery{} }

func (noDiscovery) Provider() string                           { return config.DiscoveryNone }
func (noDiscovery) Discover(context.Context) ([]string, error) { return nil, nil }

type staticDiscovery struct{ addresses []string }

// NewStaticDiscovery reports a fixed address list.
//
// A list too long to fan out to is rejected when it is used rather than here.
// Construction therefore stays infallible.
// The error reaches the caller as an unreachable roster entry.
func NewStaticDiscovery(addresses []string) Discovery {
	return staticDiscovery{addresses: addresses}
}

func (staticDiscovery) Provider() string { return config.DiscoveryStatic }

func (s staticDiscovery) Discover(context.Context) ([]string, error) {
	return normalizeAddresses(s.addresses)
}

// LookupHostFunc resolves a host to addresses.
type LookupHostFunc func(ctx context.Context, host string) ([]string, error)

// DNSOption customises a DNS discovery provider.
type DNSOption func(*dnsDiscovery)

// WithLookupHost replaces the resolver.
func WithLookupHost(f LookupHostFunc) DNSOption {
	return func(d *dnsDiscovery) { d.lookupHost = f }
}

type dnsDiscovery struct {
	name       string
	port       int
	lookupHost LookupHostFunc
}

// NewDNSDiscovery resolves name to one address per sibling pod, as a Kubernetes headless Service does.
// Each address is paired with port.
//
// Such a Service publishes ready endpoints only.
// A pod that is failing its readiness probe is absent from the result rather than reported as unreachable.
// Responses say which provider produced the roster.
// A reader can account for that.
func NewDNSDiscovery(name string, port int, opts ...DNSOption) Discovery {
	d := &dnsDiscovery{
		name:       name,
		port:       port,
		lookupHost: net.DefaultResolver.LookupHost,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *dnsDiscovery) Provider() string { return config.DiscoveryDNS }

func (d *dnsDiscovery) Discover(ctx context.Context) ([]string, error) {
	hosts, err := d.lookupHost(ctx, d.name)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", d.name, err)
	}
	addresses := make([]string, 0, len(hosts))
	for _, h := range hosts {
		// JoinHostPort brackets IPv6 literals.
		// A bare concatenation would not.
		addresses = append(addresses, net.JoinHostPort(h, strconv.Itoa(d.port)))
	}
	return normalizeAddresses(addresses)
}

// normalizeAddresses sorts and de-duplicates an address list so a roster is stable across calls.
//
// Exceeding the cap is an error rather than a truncation.
// Truncating would be by sorted address and would drop the same members on every call.
// A stable blind spot would be reported as a complete survey.
// A proxy deployment does not have this many pods.
// The realistic cause is a name that also matches something else.
// That is worth saying out loud.
func normalizeAddresses(in []string) ([]string, error) {
	out := slices.Clone(in)
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > maxDiscoveredMembers {
		return nil, fmt.Errorf("discovered %d peers, more than the %d this proxy will fan out to: "+
			"check that the name resolves only to this deployment", len(out), maxDiscoveredMembers)
	}
	return out, nil
}
