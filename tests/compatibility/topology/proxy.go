//go:build testcompatibility

package topology

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	proxyPProfPort        = 6060
	proxyReadinessTimeout = 30 * time.Second
)

// proxyHandle implements ProxyHandle for a single proxy container.
type proxyHandle struct {
	name          string
	container     testcontainers.Container
	internalAddrs []string
	hostAddr      string
	conn          *grpc.ClientConn
	logConsumer   *logConsumer
}

func (p *proxyHandle) Name() string            { return p.name }
func (p *proxyHandle) InternalAddrs() []string { return p.internalAddrs }
func (p *proxyHandle) HostAddr() string        { return p.hostAddr }

func (p *proxyHandle) FrontendClient() workflowservicev1.WorkflowServiceClient {
	return workflowservicev1.NewWorkflowServiceClient(p.conn)
}

func (p *proxyHandle) Logs() []string {
	if p.logConsumer == nil {
		return nil
	}
	return p.logConsumer.Logs()
}

func (p *proxyHandle) Stop(ctx context.Context) error {
	var errs []error

	// Step 1. Close the gRPC connection.
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Step 2. Terminate the proxy container.
	if p.container != nil {
		if err := p.container.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("terminate %s: %w", p.name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s stop: %w", p.name, errors.Join(errs...))
	}
	return nil
}

// repoRoot finds the repository root by climbing from the current file's location.
func repoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("could not determine caller file")
	}
	// filename is .../tests/compatibility/topology/proxy.go
	// climb: topology/ -> compatibility/ -> tests/ -> repo root
	dir := filepath.Dir(filename)
	root := filepath.Join(dir, "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("abs repo root: %w", err)
	}
	return abs, nil
}

var (
	proxyBuildOnce sync.Once
	proxyImageTag  string
	proxyBuildErr  error
)

func ensureProxyImage() (string, error) {
	// We want to ensure the proxy image is cached per test run to reduce test run time
	// We can't use commit sha or similar as a cache key due to local changes and multiple runs per commit during development
	// So we just use a random string!
	proxyBuildOnce.Do(func() {
		runID := fmt.Sprintf("%04x", rand.Uint32()&0xffff) //nolint:gosec
		proxyImageTag, proxyBuildErr = buildProxyImage(context.Background(), runID)
	})
	return proxyImageTag, proxyBuildErr
}

// buildProxyImage builds the proxy Docker image and returns the image tag.
func buildProxyImage(ctx context.Context, runID string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	tag := fmt.Sprintf("temporalio/s2s-proxy:compat-test-%s-%s", runtime.GOARCH, runID)
	cmd := exec.CommandContext(ctx, "docker", "build",
		"--platform", "linux/"+runtime.GOARCH, "-t", tag, root)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build proxy image: %w", err)
	}
	return tag, nil
}

// startProxy starts a single proxy container and returns a ProxyHandle.
func startProxy(ctx context.Context, spec proxySpec, topologyID, label string) (ProxyHandle, error) {
	containerName := makeContainerName(topologyID, "proxy", label)
	exposedPorts := append(append([]string(nil), spec.exposedPorts...), exposedPort(proxyPProfPort))
	req := testcontainers.ContainerRequest{
		Name:  containerName,
		Image: spec.image,
		Env:   map[string]string{"CONFIG_YML": spec.configFile.MountPath},
		Files: []testcontainers.ContainerFile{
			{Reader: strings.NewReader(spec.configFile.Content), ContainerFilePath: spec.configFile.MountPath, FileMode: 0644},
		},
		ExposedPorts:   exposedPorts,
		Networks:       []string{spec.network.Name},
		NetworkAliases: map[string][]string{spec.network.Name: {spec.networkAlias}},
		WaitingFor: wait.ForHTTP("/debug/pprof/").
			WithPort(nat.Port(exposedPort(proxyPProfPort))).
			WithStartupTimeout(proxyReadinessTimeout),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start proxy %s: %w", spec.networkAlias, err)
	}

	mappedPort, err := c.MappedPort(ctx, nat.Port(spec.exposedPorts[0]))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("get mapped port for proxy %s: %w", spec.networkAlias, err)
	}

	logs := attachLogConsumer(ctx, c)
	addr := hostAddr(mappedPort.Port())

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("dial proxy %s: %w", spec.networkAlias, err)
	}

	return &proxyHandle{
		name:          spec.name,
		container:     c,
		internalAddrs: spec.internalAddrs,
		hostAddr:      addr,
		conn:          conn,
		logConsumer:   logs,
	}, nil
}
