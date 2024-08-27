package config

import (
	"os"

	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

const (
	ConfigPathFlag             = "config"
	OutboundPortFlag           = "outbound-port"
	InboundPortFlag            = "inbound-port"
	RemoteServerRPCAddressFlag = "remote"
	LocalServerRPCAddressFlag  = "local"
	// Localhost default hostname
	LocalhostIPDefault = "127.0.0.1"
)

type (
	ConfigProvider interface {
		GetS2SProxyConfig() S2SProxyConfig
	}

	ServerConfig struct {
		// RPCAddress indicate the server address(Host:Port) for listening requests
		ListenAddress string                     `yaml:"listenAddress"`
		TLS           encryption.ServerTLSConfig `yaml:"tls"`
	}

	ClientConfig struct {
		// RPCAddress indicate the address(Host:Port) for forwarding requests
		ForwardAddress string                     `yaml:"forwardAddress"`
		TLS            encryption.ClientTLSConfig `yaml:"tls"`
	}

	ProxyConfig struct {
		Name   string       `yaml:"name"`
		Server ServerConfig `yaml:"server"`
		Client ClientConfig `yaml:"client"`
	}

	S2SProxyConfig struct {
		Inbound  ProxyConfig `yaml:"inbound"`
		Outbound ProxyConfig `yaml:"outbound"`
	}

	cliConfigProvider struct {
		ctx    *cli.Context
		config S2SProxyConfig
	}
)

func newConfigProvider(ctx *cli.Context) (ConfigProvider, error) {
	config := &cliConfigProvider{
		ctx: ctx,
	}

	if err := config.load(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *cliConfigProvider) GetS2SProxyConfig() S2SProxyConfig {
	return c.config
}

func (c *cliConfigProvider) load() error {
	configFilePath := c.ctx.String(ConfigPathFlag)
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &c.config)
	if err != nil {
		return err
	}

	return nil
}
