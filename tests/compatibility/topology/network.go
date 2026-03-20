//go:build testcompatibility

package topology

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go/network"
)

// newNetwork creates a new Docker network.
func newNetwork(ctx context.Context) (Network, error) {
	net, err := network.New(ctx)
	if err != nil {
		return Network{}, fmt.Errorf("create docker network: %w", err)
	}
	return Network{Name: net.Name}, nil
}
