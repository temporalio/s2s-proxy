package config

import "github.com/urfave/cli/v2"

type (
	ClientTLSConfigProvider interface {
		// For client authentication.
		GetCertificatePath() string
		GetKeyPath() string

		// For server authentication.
		GetServerName() string
		GetServerCAPath() string
	}

	cliClientTlsConfigProvider struct {
		ctx *cli.Context
	}
)

func (c *cliClientTlsConfigProvider) GetCertificatePath() string {
	return c.ctx.String(TlsLocalClientCertPathFlag)
}

func (c *cliClientTlsConfigProvider) GetKeyPath() string {
	return c.ctx.String(TlsLocalClientKeyPathFlag)
}

func (c *cliClientTlsConfigProvider) GetServerName() string {
	return c.ctx.String(TlsLocalServerNameFlag)
}

func (c *cliClientTlsConfigProvider) GetServerCAPath() string {
	return c.ctx.String(TlsLocalServerCAPathFlag)
}

func (c *cliClientTlsConfigProvider) isTlsEnabled() bool {
	if c.GetCertificatePath() != "" && c.GetKeyPath() != "" {
		// has valid config for client auth.
		return true
	}

	if c.GetServerName() != "" && c.GetServerCAPath() != "" {
		// has valid config for server auth.
		return true
	}

	return false
}
