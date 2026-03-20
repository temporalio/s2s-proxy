//go:build testcompatibility

package suite

import "time"

const (
	replicationTimeout  = 60 * time.Second
	replicationInterval = 2 * time.Second

	registerNamespaceTimeout  = 2 * time.Minute
	registerNamespaceInterval = 3 * time.Second

	rpcTimeout = 10 * time.Second

	writeTimeout = 15 * time.Second

	healthCheckTimeout = 30 * time.Second
)
