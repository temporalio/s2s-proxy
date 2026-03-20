//go:build testcompatibility

package topology

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// databaseHandle represents a running database for a Temporal cluster.
type databaseHandle interface {
	Name() string           // short network alias, e.g. "db-a"
	InternalAddr() string   // "alias:port" on the Docker network
	Env() map[string]string // Temporal env vars (DB, DB_PORT, POSTGRES_SEEDS, etc.)
	Stop(ctx context.Context) error
}

const (
	postgresUser       = "temporal"
	postgresPassword   = "temporal"
	postgresDBName     = "temporal"
	postgresDBType     = "postgres12"
	postgresPort       = "5432"
	postgresAuthMethod = "trust" // scram-sha-256 (default in postgres 15+) is not supported by older lib/pq
)

// startDatabase starts a postgres database container.
func startDatabase(ctx context.Context, dbImage string, net Network, containerName, networkAlias string) (databaseHandle, error) {
	req := testcontainers.ContainerRequest{
		Name:  containerName,
		Image: dbImage,
		Env: map[string]string{
			"POSTGRES_USER":             postgresUser,
			"POSTGRES_PASSWORD":         postgresPassword,
			"POSTGRES_DB":               postgresDBName,
			"POSTGRES_HOST_AUTH_METHOD": postgresAuthMethod,
		},
		Networks:       []string{net.Name},
		NetworkAliases: map[string][]string{net.Name: {networkAlias}},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start postgres %s: %w", containerName, err)
	}
	return &postgresHandle{
		container:    container,
		networkAlias: networkAlias,
	}, nil
}

// postgresHandle implements databaseHandle for a running Postgres container.
type postgresHandle struct {
	container    testcontainers.Container
	networkAlias string
}

// Name returns the short network alias, e.g. "db-a".
func (h *postgresHandle) Name() string { return h.networkAlias }

// InternalAddr returns "alias:port" for use on the Docker network.
func (h *postgresHandle) InternalAddr() string { return h.networkAlias + ":" + postgresPort }

// Env returns environment variables consumed by Temporal to connect to this database.
func (h *postgresHandle) Env() map[string]string {
	return map[string]string{
		"DB":                postgresDBType,
		"DB_PORT":           postgresPort,
		"POSTGRES_SEEDS":    h.networkAlias,
		"POSTGRES_USER":     postgresUser,
		"POSTGRES_PASSWORD": postgresPassword,
	}
}

// Stop terminates the postgres container.
func (h *postgresHandle) Stop(ctx context.Context) error {
	if err := h.container.Terminate(ctx); err != nil {
		return fmt.Errorf("terminate postgres: %w", err)
	}
	return nil
}
