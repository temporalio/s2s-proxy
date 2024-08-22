package config

import "github.com/urfave/cli/v2"

type (
	cliClientTlsConfigProvider struct {
		ctx *cli.Context
	}
)

func (c *cliClientTlsConfigProvider) GetCertificatePath() string {
	return c.ctx.String(TlsLocalClientCertPathFlag)
}

func (c *cliClientTlsConfigProvider) GetKeyPath() string {
	return c.ctx.String(TlsLocalClientCertPathFlag)
}

func (c *cliClientTlsConfigProvider) IsHostVerificationEnabled() bool {
	return c.ctx.Bool(TlsLocalIsHostVerifyEnabled)
}

func (c *cliClientTlsConfigProvider) GetServerName() string {
	return c.ctx.String(TlsLocalServerNameFlag)
}

func (c *cliClientTlsConfigProvider) GetServerCAPath() string {
	return c.ctx.String(TlsLocalServerCAPathFlag)
}

func (c *cliClientTlsConfigProvider) validate() bool {
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
