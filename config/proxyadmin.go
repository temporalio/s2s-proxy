package config

import (
	"errors"
	"fmt"
	"net"
	"slices"

	"github.com/temporalio/temporal-proxy/pkg/validation"
)

const (
	DiscoveryNone = "none"
)

var DiscoveryProviders = []string{DiscoveryNone}

func (c *ProxyAdminConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Field("listenAddress", c.ListenAddress,
			validation.When(isSet,
				validation.IsHostPort(),
				validation.When(parsesAsHostPort, isLoopback(
					"Bind it to loopback, or serve siblings through proxyAdmin.peer. The peer listener authenticates its callers.")),
			)),
		validation.WhenNested(func() bool { return c.Peer != nil }, "peer", c.Peer),
	)
}

func (p *ProxyAdminPeerConfig) Validate() error {
	tlsEnabled := p.TLS != nil && p.TLS.IsEnabled()

	rules := []validation.Rule{
		validation.Field("listenAddress", p.ListenAddress,
			validation.Required[string](),
			validation.When(isSet,
				validation.IsHostPort(),
				validation.When(parsesAsHostPort,
					validation.WhenFn(func() bool { return !tlsEnabled && !p.AllowInsecure }, isLoopback(
						"Configure proxyAdmin.peer.tls, or set proxyAdmin.peer.allowInsecure to accept it."))),
			)),
		validation.Field("discovery.provider", p.Discovery.Provider, knownDiscoveryProvider()),
	}
	if tlsEnabled {
		rules = append(rules, p.tlsRules()...)
	}

	return validation.Validate("", rules...)
}

func (p *ProxyAdminPeerConfig) tlsRules() []validation.Rule {
	return []validation.Rule{
		validation.Field("tls.certificatePath", p.TLS.CertificatePath, validation.Required[string]()),
		validation.Field("tls.keyPath", p.TLS.KeyPath, validation.Required[string]()),
		validation.Field("tls.remoteCAPath", p.TLS.RemoteCAPath, validation.Required[string]()),
		validation.Field("tls.caServerName", p.TLS.CAServerName, validation.Required[string]()),
		validation.Field("tls.skipCAVerification", p.TLS.SkipCAVerification, isFalse(
			"disables verification of every sibling this pod dials. The peer TLS set alongside it then does nothing.")),
	}
}

func knownDiscoveryProvider() validation.Check[string] {
	return func(provider string) error {
		if provider == "" || slices.Contains(DiscoveryProviders, provider) {
			return nil
		}
		return fmt.Errorf("is %q, want one of %v or empty for %q",
			provider, DiscoveryProviders, DiscoveryNone)
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

func isSet(s string) bool { return s != "" }

func parsesAsHostPort(listenAddress string) bool {
	_, _, err := net.SplitHostPort(listenAddress)
	return err == nil
}

func isLoopback(remedy string) validation.Check[string] {
	return func(listenAddress string) error {
		if loopbackListenAddress(listenAddress) {
			return nil
		}
		return fmt.Errorf("is %q, not a loopback address: this publishes an unauthenticated view "+
			"of the deployment topology to anything that can reach it. %s", listenAddress, remedy)
	}
}

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
