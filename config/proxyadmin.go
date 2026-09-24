package config

import (
	"fmt"
	"net"

	"github.com/temporalio/temporal-proxy/pkg/validation"
)

func (c *ProxyAdminConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Field("listenAddress", c.ListenAddress,
			validation.When(isSet,
				validation.IsHostPort(),
				validation.When(parsesAsHostPort, isLoopback()),
			)),
	)
}

func isSet(s string) bool { return s != "" }

func parsesAsHostPort(listenAddress string) bool {
	_, _, err := net.SplitHostPort(listenAddress)
	return err == nil
}

func isLoopback() validation.Check[string] {
	return func(listenAddress string) error {
		if loopbackListenAddress(listenAddress) {
			return nil
		}
		return fmt.Errorf("is %q, not a loopback address: this publishes an unauthenticated view "+
			"of the deployment topology to anything that can reach it", listenAddress)
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
