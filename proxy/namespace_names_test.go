package proxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

// fakeDescriber answers DescribeNamespace from names by ID, failing with err
// when it is set, and records the IDs it was asked for.
type fakeDescriber struct {
	mu    sync.Mutex
	names map[string]string
	err   error
	asked []string
}

func (f *fakeDescriber) DescribeNamespace(_ context.Context, req *workflowservice.DescribeNamespaceRequest, _ ...grpc.CallOption) (*workflowservice.DescribeNamespaceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, req.GetId())
	if f.err != nil {
		return nil, f.err
	}

	return &workflowservice.DescribeNamespaceResponse{
		NamespaceInfo: &namespace.NamespaceInfo{Id: req.GetId(), Name: f.names[req.GetId()]},
	}, nil
}

func TestNamespaceNamesResolvesAndCaches(t *testing.T) {
	d := &fakeDescriber{names: map[string]string{"id-a": "a"}}
	n := newNamespaceNames(d)

	for range 2 {
		name, err := n.LocalName(t.Context(), "id-a")
		require.NoError(t, err)
		require.Equal(t, "a", name)
	}

	require.Equal(t, []string{"id-a"}, d.asked, "the second lookup is served from the cache")
}

func TestNamespaceNamesDoesNotCacheFailures(t *testing.T) {
	d := &fakeDescriber{names: map[string]string{"id-a": "a"}, err: errors.New("not yet replicated")}
	n := newNamespaceNames(d)

	_, err := n.LocalName(t.Context(), "id-a")
	require.ErrorContains(t, err, "not yet replicated")
	require.ErrorContains(t, err, "id-a")

	d.err = nil
	name, err := n.LocalName(t.Context(), "id-a")
	require.NoError(t, err)
	require.Equal(t, "a", name)
	require.Len(t, d.asked, 2)
}

func TestNamespaceNamesRefusesAnEmptyName(t *testing.T) {
	n := newNamespaceNames(&fakeDescriber{})

	_, err := n.LocalName(t.Context(), "id-a")
	require.ErrorContains(t, err, "namespace id-a described with no name")
}

// A rename reaches the cache once the old name expires.
func TestNamespaceNamesExpires(t *testing.T) {
	d := &fakeDescriber{names: map[string]string{"id-a": "a"}}
	n := newNamespaceNames(d)
	now := time.Now()
	n.now = func() time.Time { return now }

	_, err := n.LocalName(t.Context(), "id-a")
	require.NoError(t, err)

	d.names["id-a"] = "renamed"
	now = now.Add(namespaceNameTTL - time.Second)
	name, err := n.LocalName(t.Context(), "id-a")
	require.NoError(t, err)
	require.Equal(t, "a", name, "still cached")

	now = now.Add(time.Second)
	name, err = n.LocalName(t.Context(), "id-a")
	require.NoError(t, err)
	require.Equal(t, "renamed", name)
}

// blockingDescriber holds every call until release is closed, after telling
// started, and fails it if its context ends first.
type blockingDescriber struct {
	fakeDescriber
	started chan struct{}
	release chan struct{}
}

func (b *blockingDescriber) DescribeNamespace(ctx context.Context, req *workflowservice.DescribeNamespaceRequest, opts ...grpc.CallOption) (*workflowservice.DescribeNamespaceResponse, error) {
	b.started <- struct{}{}
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return b.fakeDescriber.DescribeNamespace(ctx, req, opts...)
}

// A burst of tasks for a namespace not yet cached makes one call, which the
// first caller giving up does not end for the rest.
func TestNamespaceNamesSharesLookups(t *testing.T) {
	d := &blockingDescriber{
		fakeDescriber: fakeDescriber{names: map[string]string{"id-a": "a"}},
		started:       make(chan struct{}, 2),
		release:       make(chan struct{}),
	}
	n := newNamespaceNames(d)

	first, cancelFirst := context.WithCancel(t.Context())
	firstErr := make(chan error, 1)
	go func() {
		_, err := n.LocalName(first, "id-a")
		firstErr <- err
	}()
	<-d.started

	type result struct {
		name string
		err  error
	}
	second := make(chan result, 1)
	go func() {
		name, err := n.LocalName(t.Context(), "id-a")
		second <- result{name, err}
	}()

	cancelFirst()
	// singleflight offers no way to see a caller join, so give the second one
	// time to before the call ends. Too little shows up as a second call.
	time.Sleep(50 * time.Millisecond)
	close(d.release)

	got := <-second
	require.NoError(t, got.err)
	require.Equal(t, "a", got.name)
	require.NoError(t, <-firstErr, "the shared call outlives the first caller's context")
	require.Equal(t, []string{"id-a"}, d.asked)
}
