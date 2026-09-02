package config

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"

	"github.com/temporalio/temporal-proxy/pkg/validation"
)

// Discovery provider names. Empty is equivalent to DiscoveryNone.
const (
	DiscoveryNone   = "none"
	DiscoveryDNS    = "dns"
	DiscoveryStatic = "static"
)

// DiscoveryProviders lists every provider name Validate accepts, for error messages.
var DiscoveryProviders = []string{DiscoveryNone, DiscoveryDNS, DiscoveryStatic}

// Validate reports configuration that can never work.
// A typo fails startup rather than leaving a listener that silently never starts.
//
// Failures that depend on the environment rather than the config are handled at runtime instead.
// A port already in use and a name that does not resolve are both of that kind.
func (c *ProxyAdminConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Field("listenAddress", c.ListenAddress,
			validation.When(isSet, validation.IsHostPort(), isLoopback(
				"Bind it to loopback, or serve siblings through proxyAdmin.peer, which authenticates its callers."))),
		validation.WhenNested(func() bool { return c.Peer != nil }, "peer", c.Peer),
	)
}

func (p *ProxyAdminPeerConfig) Validate() error {
	// TLSConfig.IsEnabled is true when only caServerName is set.
	// The fields below are therefore required individually rather than taken as a group.
	tlsEnabled := p.TLS != nil && p.TLS.IsEnabled()

	rules := []validation.Rule{
		validation.Field("listenAddress", p.ListenAddress,
			validation.Required[string](), validation.IsHostPort(),
			validation.WhenFn(func() bool { return !tlsEnabled && !p.AllowInsecure }, isLoopback(
				"Configure proxyAdmin.peer.tls, or set proxyAdmin.peer.allowInsecure to accept it."))),
	}
	if tlsEnabled {
		rules = append(rules, p.tlsRules()...)
	}
	rules = append(rules,
		validation.Field("discovery.dns.port", p.Discovery.DNS.Port,
			validation.WhenFn(p.discoveryPortUnresolvable, validation.Required[int]())),
		validation.Nested("discovery", &p.Discovery),
	)

	return validation.Validate("", rules...)
}

// tlsRules validates the peer TLS block.
// That block serves the peer listener and dials every sibling.
func (p *ProxyAdminPeerConfig) tlsRules() []validation.Rule {
	return []validation.Rule{
		validation.Field("tls.certificatePath", p.TLS.CertificatePath, validation.Required[string]()),
		validation.Field("tls.keyPath", p.TLS.KeyPath, validation.Required[string]()),
		// The peer listener verifies its callers, unlike the mux.
		// It needs the CA to verify them against.
		// Without this the listener fails to build and is skipped at startup.
		// The only symptom is one log line plus every sibling reporting this pod unreachable.
		validation.Field("tls.remoteCAPath", p.TLS.RemoteCAPath, validation.Required[string]()),
		// Peers are dialed by IP.
		// This must name a SAN that every pod's certificate carries.
		validation.Field("tls.caServerName", p.TLS.CAServerName, validation.Required[string]()),
		validation.Field("tls.skipCAVerification", p.TLS.SkipCAVerification, isFalse(
			"disables verification of every sibling this pod dials, which defeats the peer TLS it is set alongside")),
	}
}

// discoveryPortUnresolvable reports whether the dns provider has no port to dial siblings on.
// Neither discovery.dns.port nor the peer listen address supplies one in that case.
func (p *ProxyAdminPeerConfig) discoveryPortUnresolvable() bool {
	return p.Discovery.Provider == DiscoveryDNS && p.PeerPort() == 0
}

func (d *DiscoveryConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Field("provider", d.Provider, knownDiscoveryProvider()),
		// Blocks belonging to an unselected provider are deliberately not validated.
		// Layered configuration deep-merges and cannot delete keys.
		// Switching provider leaves the previous provider's block behind.
		// It must stay inert.
		validation.WhenRules(func() bool { return d.Provider == DiscoveryDNS },
			validation.Field("dns.name", d.DNS.Name, validation.Required[string]()),
		),
		validation.WhenRules(func() bool { return d.Provider == DiscoveryStatic },
			validation.Field("static.addresses", d.Static.Addresses, nonEmpty[string]()),
		),
	)
}

func isSet(s string) bool { return s != "" }

func nonEmpty[V any]() validation.Check[[]V] {
	return func(vs []V) error {
		if len(vs) == 0 {
			return errors.New("is required")
		}
		return nil
	}
}

func knownDiscoveryProvider() validation.Check[string] {
	return func(provider string) error {
		if provider == "" || slices.Contains(DiscoveryProviders, provider) {
			return nil
		}
		return fmt.Errorf("is %q, want one of %v", provider, DiscoveryProviders)
	}
}

// isLoopback rejects an address reachable from outside this host.
// An unparseable or name-based host is rejected too.
// The check errs toward making the operator state their intent.
func isLoopback(remedy string) validation.Check[string] {
	return func(listenAddress string) error {
		if loopbackListenAddress(listenAddress) {
			return nil
		}
		return fmt.Errorf("is %q, which is not loopback: this publishes an unauthenticated view "+
			"of the deployment topology to anything that can reach it. %s", listenAddress, remedy)
	}
}

func isFalse(because string) validation.Check[bool] {
	return func(v bool) error {
		if !v {
			return nil
		}
		return errors.New(because)
	}
}

// peerPort returns the port of a host:port listen address, or 0 when the address binds an arbitrary port.
func peerPort(listenAddress string) (int, error) {
	_, portStr, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return 0, err
	}
	if portStr == "" || portStr == "0" {
		return 0, nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return port, nil
}

// loopbackListenAddress reports whether an address only accepts connections from this host.
func loopbackListenAddress(listenAddress string) bool {
	host, _, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// PeerPort returns the port the peer listener binds, for defaulting discovery ports.
func (p ProxyAdminPeerConfig) PeerPort() int {
	port, err := peerPort(p.ListenAddress)
	if err != nil {
		return 0
	}
	return port
}
