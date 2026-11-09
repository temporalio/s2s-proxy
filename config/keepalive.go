package config

import (
	"fmt"
	"time"

	"go.temporal.io/proxy/pkg/validation"
	"google.golang.org/grpc/keepalive"
)

// MinClientKeepaliveTime is the shortest client ping interval grpc-go honors. It raises anything shorter to this.
const MinClientKeepaliveTime = 10 * time.Second

type (
	// KeepaliveConfig configures HTTP/2 keepalive on the gRPC client and the gRPC server the proxy builds from one
	// ClusterDefinition. Leaving the block, or any field in it, unset keeps grpc-go's default.
	//
	// On a tcp definition, Client applies to the tcpClient connection and Server to the tcpServer listener. On a
	// mux-client or mux-server definition, both apply to the gRPC traffic inside the mux: Client to calls this proxy
	// makes to the peer proxy, Server to calls the peer proxy makes to this one.
	//
	// A client must not ping more often than the server on the other end allows, or the server closes the
	// connection with GOAWAY ENHANCE_YOUR_CALM "too_many_pings". Configure the server side first.
	KeepaliveConfig struct {
		Client *ClientKeepaliveConfig `yaml:"client"`
		Server *ServerKeepaliveConfig `yaml:"server"`
	}

	// ClientKeepaliveConfig maps to grpc-go's keepalive.ClientParameters.
	ClientKeepaliveConfig struct {
		// Time is how long the client goes without receiving anything before it pings the server. Zero means never,
		// which is grpc-go's default. Must be at least MinClientKeepaliveTime when set.
		Time time.Duration `yaml:"time"`
		// Timeout is how long the client waits for a ping's answer before it closes the connection. Zero means
		// grpc-go's default, 20s.
		Timeout time.Duration `yaml:"timeout"`
		// PermitWithoutStream lets the client ping when no RPC or stream is open on the connection.
		PermitWithoutStream bool `yaml:"permitWithoutStream"`
	}

	// ServerKeepaliveConfig maps to grpc-go's keepalive.ServerParameters and keepalive.EnforcementPolicy.
	ServerKeepaliveConfig struct {
		// Time is how long the server goes without receiving anything before it pings the client. Zero means
		// grpc-go's default, 2h.
		Time time.Duration `yaml:"time"`
		// Timeout is how long the server waits for a ping's answer before it closes the connection. Zero means
		// grpc-go's default, 20s.
		Timeout time.Duration `yaml:"timeout"`
		// EnforcementPolicy sets which client pings the server accepts. Unset keeps grpc-go's default: at most one
		// ping every 5m, and only while a stream is open.
		EnforcementPolicy *KeepaliveEnforcementPolicyConfig `yaml:"enforcementPolicy"`
	}

	// KeepaliveEnforcementPolicyConfig maps to grpc-go's keepalive.EnforcementPolicy.
	KeepaliveEnforcementPolicyConfig struct {
		// MinTime is the shortest interval between client pings the server accepts. Zero means grpc-go's default, 5m.
		MinTime time.Duration `yaml:"minTime"`
		// PermitWithoutStream accepts client pings when no RPC or stream is open on the connection.
		PermitWithoutStream bool `yaml:"permitWithoutStream"`
	}
)

// ClientKeepalive returns the client keepalive settings for this definition, or nil when none are configured.
func (c ClusterDefinition) ClientKeepalive() *ClientKeepaliveConfig {
	if c.Keepalive == nil {
		return nil
	}
	return c.Keepalive.Client
}

// ServerKeepalive returns the server keepalive settings for this definition, or nil when none are configured.
func (c ClusterDefinition) ServerKeepalive() *ServerKeepaliveConfig {
	if c.Keepalive == nil {
		return nil
	}
	return c.Keepalive.Server
}

// Parameters converts c to grpc-go's client parameters.
func (c *ClientKeepaliveConfig) Parameters() keepalive.ClientParameters {
	return keepalive.ClientParameters{
		Time:                c.Time,
		Timeout:             c.Timeout,
		PermitWithoutStream: c.PermitWithoutStream,
	}
}

// Parameters converts c to grpc-go's server parameters.
func (c *ServerKeepaliveConfig) Parameters() keepalive.ServerParameters {
	return keepalive.ServerParameters{
		Time:    c.Time,
		Timeout: c.Timeout,
	}
}

// Policy converts p to grpc-go's enforcement policy.
func (p *KeepaliveEnforcementPolicyConfig) Policy() keepalive.EnforcementPolicy {
	return keepalive.EnforcementPolicy{
		MinTime:             p.MinTime,
		PermitWithoutStream: p.PermitWithoutStream,
	}
}

func (k *KeepaliveConfig) Validate() error {
	if k == nil {
		return nil
	}
	var rules []validation.Rule
	if k.Client != nil {
		rules = append(rules,
			validation.Field("client.time", k.Client.Time, func(d time.Duration) error {
				if d != 0 && d < MinClientKeepaliveTime {
					return fmt.Errorf("must be 0 or at least %s, got %s", MinClientKeepaliveTime, d)
				}
				return nil
			}),
			validation.Field("client.timeout", k.Client.Timeout, validation.GTE[time.Duration](0)),
		)
	}
	if k.Server != nil {
		rules = append(rules,
			validation.Field("server.time", k.Server.Time, validation.GTE[time.Duration](0)),
			validation.Field("server.timeout", k.Server.Timeout, validation.GTE[time.Duration](0)),
		)
		if k.Server.EnforcementPolicy != nil {
			rules = append(rules,
				validation.Field("server.enforcementPolicy.minTime", k.Server.EnforcementPolicy.MinTime,
					validation.GTE[time.Duration](0)))
		}
	}
	return validation.Validate("", rules...)
}
