//go:build testcompatibility

package topology

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	adminservicev1 "go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications"
)

const (
	serverGRPCPort        = 7233
	schemaSetupTimeout    = 90 * time.Second
	serverConfigMountPath = "/etc/temporal/config/config_template.yaml"
)

type clusterHandle struct {
	name         string
	hostAddr     string
	internalAddr string

	conn *grpc.ClientConn

	db          databaseHandle
	container   testcontainers.Container
	logConsumer *logConsumer
}

func (c *clusterHandle) Name() string         { return c.name }
func (c *clusterHandle) HostAddr() string     { return c.hostAddr }
func (c *clusterHandle) InternalAddr() string { return c.internalAddr }

func (c *clusterHandle) FrontendClient() workflowservicev1.WorkflowServiceClient {
	return workflowservicev1.NewWorkflowServiceClient(c.conn)
}

func (c *clusterHandle) AdminClient() adminservicev1.AdminServiceClient {
	return adminservicev1.NewAdminServiceClient(c.conn)
}

func (c *clusterHandle) Logs() []string {
	if c.logConsumer == nil {
		return nil
	}
	return c.logConsumer.Logs()
}

// Stop the cluster.
func (c *clusterHandle) Stop(ctx context.Context) error {

	// Step 1. Close the gRPC connection.
	var errs []error
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Step 2. Terminate the Temporal Container.
	if c.container != nil {
		if err := c.container.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("terminate server: %w", err))
		}
	}

	// Step 3. Stop the database container.
	if c.db != nil {
		if err := c.db.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop database: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cluster %s stop: %w", c.name, errors.Join(errs...))
	}
	return nil
}

// templateData is used for rendering templated files.
// Add attributes here to use in templating (eg server config, server setup).
type templateData struct {
	ClusterName            string
	DatabaseAlias          string
	NetworkAlias           string
	InitialFailoverVersion int
}

func renderTemplate(name, tmpl string, data any) string {
	t := template.Must(
		// Use [[ ]] as our substitution delimiters to avoid clashes with yaml's {{}} substitution
		template.New(name).Delims("[[", "]]").Parse(tmpl),
	)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("render %s: %v", name, err))
	}
	return buf.String()
}

func runSchemaSetup(ctx context.Context, adminToolsImage, script string, network Network) error {
	req := testcontainers.ContainerRequest{
		Image:      adminToolsImage,
		Networks:   []string{network.Name},
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd:        []string{script},
		WaitingFor: wait.ForLog("temporal schema setup complete").WithStartupTimeout(schemaSetupTimeout),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("schema setup container: %w", err)
	}

	return c.Terminate(ctx)
}

// makeContainerName returns a unique Docker container name for this topology run.
func makeContainerName(topologyID, kind, label string) string {
	return fmt.Sprintf("s2sproxy-compat-%s-%s-%s", topologyID, kind, label)
}

// startCluster starts the Temporal Cluster (including dependencies).
func startCluster(
	ctx context.Context,
	spec specifications.ClusterSpec,
	net Network,
	label string, // label is the positional key ("a", "b", ...)
	topologyID string, // unique identifier for this topology instance to dedupe docker resources across parallel test runs
) (ClusterHandle, error) {
	name := "cluster-" + label
	networkAlias := "temporal-" + label

	// initialFailoverVersion uses label to derive monotonically increasing version numbers across clusters, e.g. "a" -> 1, "b" -> 2, etc.
	initialFailoverVersion := int(label[0]-'a') + 1

	// Step 1. Start the database container.
	dbContainerName := makeContainerName(topologyID, "db", label)
	db, err := startDatabase(ctx, spec.Database.Image, net, dbContainerName, "db-"+label)
	if err != nil {
		return nil, fmt.Errorf("start database for %s: %w", name, err)
	}

	// Step 2. Prepare cluster configuration.
	dbAlias := db.Name()

	env := make(map[string]string)
	for k, v := range db.Env() {
		env[k] = v
	}
	env["BIND_ON_IP"] = "0.0.0.0"

	data := templateData{
		ClusterName:            name,
		DatabaseAlias:          dbAlias,
		NetworkAlias:           networkAlias,
		InitialFailoverVersion: initialFailoverVersion,
	}

	configContent := renderTemplate("server-config", spec.Server.ConfigTemplate, data)
	schemaScript := renderTemplate("schema-setup", spec.Server.SetupSchemaTemplate, data)

	// Step 3. Run schema setup, then start the Temporal server container.
	serverContainerName := makeContainerName(topologyID, "temporal", label)
	if err := runSchemaSetup(ctx, spec.Server.AdminToolsImage, schemaScript, net); err != nil {
		_ = db.Stop(ctx)
		return nil, fmt.Errorf("schema setup for %s: %w", networkAlias, err)
	}

	serverReq := testcontainers.ContainerRequest{
		Name:  serverContainerName,
		Image: spec.Server.Image,
		Env:   env,
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(configContent),
				ContainerFilePath: serverConfigMountPath,
				FileMode:          0644,
			},
		},
		ExposedPorts:   []string{exposedPort(serverGRPCPort)},
		Networks:       []string{net.Name},
		NetworkAliases: map[string][]string{net.Name: {networkAlias}},
		WaitingFor:     wait.ForListeningPort(nat.Port(exposedPort(serverGRPCPort))).WithStartupTimeout(3 * time.Minute),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: serverReq,
		Started:          true,
	})
	if err != nil {
		_ = db.Stop(ctx)
		return nil, fmt.Errorf("start temporal %s: %w", networkAlias, err)
	}

	mappedPort, err := c.MappedPort(ctx, nat.Port(exposedPort(serverGRPCPort)))
	if err != nil {
		_ = c.Terminate(ctx)
		_ = db.Stop(ctx)
		return nil, fmt.Errorf("get mapped port for temporal %s: %w", networkAlias, err)
	}

	lc := attachLogConsumer(ctx, c)
	hostAddr := hostAddr(mappedPort.Port())
	internalAddr := containerAddr(networkAlias, serverGRPCPort)

	// Step 4. Create a gRPC client connection to the Temporal server.
	conn, err := grpc.NewClient(hostAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = c.Terminate(ctx)
		_ = db.Stop(ctx)
		return nil, fmt.Errorf("dial gRPC for %s: %w", name, err)
	}

	return &clusterHandle{
		name:         name,
		hostAddr:     hostAddr,
		internalAddr: internalAddr,
		conn:         conn,
		db:           db,
		container:    c,
		logConsumer:  lc,
	}, nil
}
