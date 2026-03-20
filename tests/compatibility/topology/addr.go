//go:build testcompatibility

package topology

import "fmt"

// listenAddr returns a bind address for the given port, e.g. "0.0.0.0:6233".
func listenAddr(port int) string { return fmt.Sprintf("0.0.0.0:%d", port) }

// exposedPort returns the testcontainers exposed-port string, e.g. "6233/tcp".
func exposedPort(port int) string { return fmt.Sprintf("%d/tcp", port) }

// containerAddr returns a Docker-network address, e.g. "proxy-a:6233".
func containerAddr(alias string, port int) string { return fmt.Sprintf("%s:%d", alias, port) }

// hostAddr returns a host-side dial address for a mapped port, e.g. "127.0.0.1:32768".
func hostAddr(port string) string { return "127.0.0.1:" + port }
