package testutil

import (
	"fmt"
	"net"
)

// GetFreePort returns an available TCP port by listening on localhost:0.
// This is useful for tests that need to allocate ports dynamically.
func GetFreePort() int {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(fmt.Sprintf("failed to get free port: %v", err))
	}
	defer func() {
		if err := l.Close(); err != nil {
			fmt.Printf("Failed to close listener: %v\n", err)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}
