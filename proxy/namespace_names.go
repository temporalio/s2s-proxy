package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/metrics"
)

const (
	// namespaceNameTTL is how long a resolved name is trusted. IDs never change,
	// but a namespace can be renamed, and this bounds how long the old name is
	// used for.
	namespaceNameTTL = 5 * time.Minute
	// namespaceLookupTimeout bounds one DescribeNamespace call. The call is
	// shared by every caller waiting on the same ID, so it does not run on any
	// one caller's deadline.
	namespaceLookupTimeout = 10 * time.Second
)

type (
	// namespaceDescriber is the part of the workflow service namespaceNames
	// calls.
	namespaceDescriber interface {
		DescribeNamespace(context.Context, *workflowservice.DescribeNamespaceRequest, ...grpc.CallOption) (*workflowservice.DescribeNamespaceResponse, error)
	}

	// namespaceNames resolves namespace IDs to the names the local cluster knows
	// them by, for binding admin payloads, which carry IDs only. Names are cached
	// for namespaceNameTTL. Failures are not, so a namespace still replicating in
	// resolves as soon as it exists. Concurrent lookups of one ID share a call.
	//
	// The cache is unbounded: it holds the namespaces one connection carries.
	namespaceNames struct {
		client namespaceDescriber
		now    func() time.Time
		group  singleflight.Group

		mu    sync.Mutex
		names map[string]cachedName
	}

	cachedName struct {
		name    string
		expires time.Time
	}
)

func newNamespaceNames(client namespaceDescriber) *namespaceNames {
	return &namespaceNames{client: client, now: time.Now, names: map[string]cachedName{}}
}

// LocalName returns the local name of the namespace with ID id.
func (n *namespaceNames) LocalName(ctx context.Context, id string) (string, error) {
	if name, ok := n.cached(id); ok {
		return name, nil
	}

	name, err, _ := n.group.Do(id, func() (any, error) {
		return n.describe(ctx, id)
	})
	if err != nil {
		return "", err
	}

	return name.(string), nil
}

func (n *namespaceNames) cached(id string) (string, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	c, ok := n.names[id]
	if !ok || !n.now().Before(c.expires) {
		return "", false
	}

	return c.name, true
}

// describe asks the local cluster for id's name and caches it. It keeps ctx's
// values, the caller's credentials among them, but not its cancellation, since
// other callers may be waiting on the same call.
func (n *namespaceNames) describe(ctx context.Context, id string) (string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), namespaceLookupTimeout)
	defer cancel()

	resp, err := n.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{Id: id})
	if err != nil {
		return "", fmt.Errorf("failed to describe namespace %q: %w", id, err)
	}

	name := resp.GetNamespaceInfo().GetName()
	if name == "" {
		return "", errors.New("namespace " + id + " described with no name")
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	n.names[id] = cachedName{name: name, expires: n.now().Add(namespaceNameTTL)}

	return name, nil
}

// namespaceObserver counts the namespace IDs that failed to resolve and logs
// them, throttled, since every payload of such a namespace is reported.
type namespaceObserver struct {
	logger log.Logger
}

func newNamespaceObserver(logger log.Logger) namespaceObserver {
	return namespaceObserver{logger: log.NewThrottledLogger(logger, func() float64 { return 1 })}
}

func (o namespaceObserver) UnresolvedNamespace(id string, err error) {
	metrics.EncryptionUnresolvedNamespaces.WithLabelValues(id).Inc()
	o.logger.Warn("Binding admin payloads to the replication key: their namespace ID did not resolve",
		tag.NewStringTag("namespace_id", id), tag.Error(err))
}
