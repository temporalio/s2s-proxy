package adminplane

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/config"
)

func TestNewDiscoverySelectsProvider(t *testing.T) {
	t.Run("empty provider means none", func(t *testing.T) {
		d, err := NewDiscovery(config.DiscoveryConfig{}, 9234)
		require.NoError(t, err)
		require.Equal(t, config.DiscoveryNone, d.Provider())

		addresses, err := d.Discover(context.Background())
		require.NoError(t, err)
		require.Empty(t, addresses)
	})

	// Layered configuration deep-merges and cannot delete keys.
	// Switching provider leaves the previous provider's block behind.
	// The selected provider must ignore it entirely.
	t.Run("an unselected provider's block is ignored", func(t *testing.T) {
		d, err := NewDiscovery(config.DiscoveryConfig{
			Provider: config.DiscoveryStatic,
			DNS:      config.DNSDiscoveryConfig{Name: "leftover.svc.cluster.local"},
			Static:   config.StaticDiscoveryConfig{Addresses: []string{"b:9234", "a:9234"}},
		}, 9234)
		require.NoError(t, err)
		require.Equal(t, config.DiscoveryStatic, d.Provider())

		addresses, err := d.Discover(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"a:9234", "b:9234"}, addresses)
	})

	t.Run("unknown provider is rejected", func(t *testing.T) {
		_, err := NewDiscovery(config.DiscoveryConfig{Provider: "carrier-pigeon"}, 9234)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown discovery provider")
	})

	// The rules for a usable provider block live in config.DiscoveryConfig.Validate.
	// That runs first.
	// This package only reports what config cannot know.

	t.Run("dns port falls back to the peer listen port", func(t *testing.T) {
		d, err := NewDiscovery(config.DiscoveryConfig{
			Provider: config.DiscoveryDNS,
			DNS:      config.DNSDiscoveryConfig{Name: "peers"},
		}, 9234)
		require.NoError(t, err)

		dns, ok := d.(*dnsDiscovery)
		require.True(t, ok)
		require.Equal(t, 9234, dns.port)
	})
}

func TestDNSDiscovery(t *testing.T) {
	lookup := func(hosts []string, err error) LookupHostFunc {
		return func(context.Context, string) ([]string, error) { return hosts, err }
	}

	t.Run("no endpoints is not an error", func(t *testing.T) {
		d := NewDNSDiscovery("peers", 9234, WithLookupHost(lookup(nil, nil)))
		addresses, err := d.Discover(context.Background())
		require.NoError(t, err)
		require.Empty(t, addresses)
	})

	t.Run("resolution failure is reported", func(t *testing.T) {
		d := NewDNSDiscovery("peers", 9234, WithLookupHost(lookup(nil, errors.New("nxdomain"))))
		_, err := d.Discover(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "peers")
	})

	t.Run("results are sorted and de-duplicated", func(t *testing.T) {
		d := NewDNSDiscovery("peers", 9234,
			WithLookupHost(lookup([]string{"10.0.0.2", "10.0.0.1", "10.0.0.2"}, nil)))
		addresses, err := d.Discover(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"10.0.0.1:9234", "10.0.0.2:9234"}, addresses)
	})

	// A bare concatenation would produce "::1:9234".
	// That does not parse.
	t.Run("IPv6 literals are bracketed", func(t *testing.T) {
		d := NewDNSDiscovery("peers", 9234, WithLookupHost(lookup([]string{"::1"}, nil)))
		addresses, err := d.Discover(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"[::1]:9234"}, addresses)
	})

	// A name pointing at the wrong service should not fan out across the whole cluster.
	// It is reported rather than truncated.
	// Truncating by sorted address drops the same members every time.
	// A stable blind spot reads as a complete survey.
	t.Run("too many peers is an error, not a truncation", func(t *testing.T) {
		var hosts []string
		for i := 0; i < maxDiscoveredMembers*2; i++ {
			hosts = append(hosts, "10.1."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256))
		}
		d := NewDNSDiscovery("peers", 9234, WithLookupHost(lookup(hosts, nil)))
		_, err := d.Discover(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "more than the")
	})
}
