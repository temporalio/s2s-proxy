// fvi-probe exercises real local clusters through a target-FVI proxy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	replicationpb "go.temporal.io/api/replication/v1"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

var ns = "fvi-prototype"

const tq = "fvi-prototype-worker"

type snapshot struct {
	Stage               string  `json:"stage"`
	SourceVersion       int64   `json:"source_version"`
	CloudVersion        int64   `json:"cloud_version"`
	SourceActive        string  `json:"source_active"`
	CloudActive         string  `json:"cloud_active"`
	SourceEventVersions []int64 `json:"source_event_versions,omitempty"`
	CloudEventVersions  []int64 `json:"cloud_event_versions,omitempty"`
}
type endpoint struct {
	admin  adminservice.AdminServiceClient
	sdk    client.Client
	system client.Client
	conn   *grpc.ClientConn
}

func main() {
	if value := os.Getenv("FVI_NAMESPACE"); value != "" {
		ns = value
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func counter(ctx workflow.Context) (int, error) {
	count := 0
	if err := workflow.SetQueryHandler(ctx, "count", func() (int, error) { return count, nil }); err != nil {
		return 0, err
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second})
	signals := workflow.GetSignalChannel(ctx, "increment")
	for {
		var value int
		signals.Receive(ctx, &value)
		if value < 0 {
			return count, nil
		}
		var result int
		if err := workflow.ExecuteActivity(ctx, echo, value).Get(ctx, &result); err != nil {
			return count, err
		}
		if err := workflow.Sleep(ctx, 20*time.Millisecond); err != nil {
			return count, err
		}
		count += result
	}
}
func echo(_ context.Context, value int) (int, error) { return value, nil }
func connect(addr string) (*endpoint, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	c, err := client.Dial(client.Options{HostPort: addr, Namespace: ns})
	if err != nil {
		return nil, err
	}
	sys, err := client.Dial(client.Options{HostPort: addr, Namespace: "temporal-system"})
	if err != nil {
		return nil, err
	}
	return &endpoint{admin: adminservice.NewAdminServiceClient(conn), sdk: c, system: sys, conn: conn}, nil
}
func getNS(ctx context.Context, e *endpoint) (*adminservice.GetNamespaceResponse, error) {
	return e.admin.GetNamespace(ctx, &adminservice.GetNamespaceRequest{Attributes: &adminservice.GetNamespaceRequest_Namespace{Namespace: ns}})
}
func wait(ctx context.Context, what string, fn func() (bool, error)) error {
	var last error
	for {
		ok, err := fn()
		if ok && err == nil {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting %s: %w (last %v)", what, ctx.Err(), last)
		case <-time.After(300 * time.Millisecond):
		}
	}
}
func newWorker(e *endpoint) (worker.Worker, error) {
	w := worker.New(e.sdk, tq, worker.Options{})
	w.RegisterWorkflow(counter)
	w.RegisterActivity(echo)
	return w, w.Start()
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s, err := connect("127.0.0.1:7233")
	if err != nil {
		return err
	}
	defer s.sdk.Close()
	defer s.system.Close()
	defer func() { _ = s.conn.Close() }()
	c, err := connect("127.0.0.1:8233")
	if err != nil {
		return err
	}
	defer c.sdk.Close()
	defer c.system.Close()
	defer func() { _ = c.conn.Close() }()
	for _, pair := range []struct {
		e     *endpoint
		proxy string
	}{{s, "127.0.0.1:38233"}, {c, "127.0.0.1:37233"}} {
		_, err = pair.e.admin.AddOrUpdateRemoteCluster(ctx, &adminservice.AddOrUpdateRemoteClusterRequest{FrontendAddress: pair.proxy, EnableRemoteClusterConnection: true})
		if err != nil {
			return fmt.Errorf("connect clusters: %w", err)
		}
	}
	// Wait for cluster metadata caches to see the peer.
	time.Sleep(4 * time.Second)
	_, err = s.sdk.WorkflowService().RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{Namespace: ns, WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour), IsGlobalNamespace: true, ActiveClusterName: "cluster-fvi-s", Clusters: []*replicationpb.ClusterReplicationConfig{{ClusterName: "cluster-fvi-s"}, {ClusterName: "cluster-fvi-c"}}})
	var exists *serviceerror.NamespaceAlreadyExists
	if err != nil && !errors.As(err, &exists) {
		return err
	}
	if err = wait(ctx, "namespace replication", func() (bool, error) { r, e := getNS(ctx, c); return e == nil && r.GetFailoverVersion() >= 1, e }); err != nil {
		return err
	}
	// Establish the discussion's old version 8 without changing any server code:
	// S generates C's initial epoch 2, then a destination-side switch on S generates 8.
	sr, err := getNS(ctx, s)
	if err != nil {
		return err
	}
	if sr.FailoverVersion == 1 {
		_, err = s.sdk.WorkflowService().UpdateNamespace(ctx, &workflowservice.UpdateNamespaceRequest{Namespace: ns, ReplicationConfig: &replicationpb.NamespaceReplicationConfig{ActiveClusterName: "cluster-fvi-c"}})
		if err != nil {
			return err
		}
		if err = wait(ctx, "seed cloud epoch 2", func() (bool, error) { r, e := getNS(ctx, c); return e == nil && r.FailoverVersion == 2, e }); err != nil {
			return err
		}
	}
	sr, err = getNS(ctx, s)
	if err != nil {
		return err
	}
	if sr.FailoverVersion == 2 {
		_, err = s.sdk.WorkflowService().UpdateNamespace(ctx, &workflowservice.UpdateNamespaceRequest{Namespace: ns, ReplicationConfig: &replicationpb.NamespaceReplicationConfig{ActiveClusterName: "cluster-fvi-s"}})
		if err != nil {
			return err
		}
	}
	if err = wait(ctx, "seed namespace version 8", func() (bool, error) { r, e := getNS(ctx, c); return e == nil && r.FailoverVersion == 8, e }); err != nil {
		return err
	}
	id := fmt.Sprintf("fvi-counter-%d", time.Now().Unix())
	histories := func(e *endpoint) ([]int64, error) {
		direct := metadata.AppendToOutgoingContext(ctx, "xdc-redirection", "false")
		it := e.sdk.GetWorkflowHistory(direct, id, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
		var versions []int64
		var last int64 = -1
		for it.HasNext() {
			ev, err := it.Next()
			if err != nil {
				return nil, err
			}
			if ev.Version != last {
				versions = append(versions, ev.Version)
				last = ev.Version
			}
		}
		return versions, nil
	}
	record := func(stage string) error {
		sr, e := getNS(ctx, s)
		if e != nil {
			return e
		}
		cr, e := getNS(ctx, c)
		if e != nil {
			return e
		}
		sv, _ := histories(s)
		cv, _ := histories(c)
		return json.NewEncoder(os.Stdout).Encode(snapshot{stage, sr.FailoverVersion, cr.FailoverVersion, sr.GetReplicationConfig().GetActiveClusterName(), cr.GetReplicationConfig().GetActiveClusterName(), sv, cv})
	}
	sw, err := newWorker(s)
	if err != nil {
		return err
	}
	run, err := s.sdk.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: id, TaskQueue: tq}, counter)
	if err != nil {
		sw.Stop()
		return err
	}
	send := func(e *endpoint, count int) error {
		if err := e.sdk.SignalWorkflow(ctx, id, "", "increment", 1); err != nil {
			return err
		}
		return wait(ctx, "workflow count", func() (bool, error) {
			v, err := e.sdk.QueryWorkflow(ctx, id, "", "count")
			if err != nil {
				return false, err
			}
			var n int
			err = v.Get(&n)
			return n == count, err
		})
	}
	if err = send(s, 1); err != nil {
		sw.Stop()
		return err
	}
	if err = wait(ctx, "initial history on cloud", func() (bool, error) { v, e := histories(c); return len(v) > 0, e }); err != nil {
		sw.Stop()
		return err
	}
	if err = record("before handover"); err != nil {
		sw.Stop()
		return err
	}
	sw.Stop()
	handover := func(from *endpoint, target string, iteration int) error {
		r, err := from.system.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: fmt.Sprintf("fvi-handover-%d-%s", time.Now().UnixNano(), target), TaskQueue: "default-worker-tq"}, "namespace-handover", map[string]any{"Namespace": ns, "RemoteCluster": target, "AllowedLaggingSeconds": 5, "AllowedLaggingTasks": 0, "HandoverTimeoutSeconds": 30})
		if err != nil {
			return err
		}
		if err = r.Get(ctx, nil); err != nil {
			return fmt.Errorf("handover %d: %w", iteration, err)
		}
		return nil
	}
	if err = handover(s, "cluster-fvi-c", 1); err != nil {
		return err
	}
	if os.Getenv("FVI_CONTROL") == "1" {
		if err = wait(ctx, "unmodified cloud cutover version", func() (bool, error) { r, e := getNS(ctx, c); return e == nil && r.FailoverVersion == 9, e }); err != nil {
			return err
		}
		if err = record("negative control without namespace rewrite"); err != nil {
			return err
		}
		var controlErr error
		err = wait(ctx, "negative control after namespace cache catches up", func() (bool, error) {
			direct, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(ctx, "xdc-redirection", "false"), 5*time.Second)
			defer cancel()
			_, controlErr = workflowservice.NewWorkflowServiceClient(c.conn).StartWorkflowExecution(direct, &workflowservice.StartWorkflowExecutionRequest{Namespace: ns, WorkflowId: id + "-control", WorkflowType: &commonpb.WorkflowType{Name: "counter"}, TaskQueue: &taskqueuepb.TaskQueue{Name: tq}, RequestId: "fvi-negative-control"})
			if controlErr == nil {
				return false, fmt.Errorf("control unexpectedly accepted invalid owner version")
			}
			return status.Code(controlErr) == codes.FailedPrecondition && strings.Contains(controlErr.Error(), "unknown-cluster-9"), nil
		})
		if err != nil {
			return err
		}
		fmt.Printf("PASS negative control: Cloud rejected version 9 with %T: %v\n", controlErr, controlErr)
		return nil
	}
	if err = wait(ctx, "cloud cutover version", func() (bool, error) { r, e := getNS(ctx, c); return e == nil && r.GetFailoverVersion() == 12, e }); err != nil {
		return err
	}
	if err = record("after S to C handover before cloud writes"); err != nil {
		return err
	}
	cw, err := newWorker(c)
	if err != nil {
		return err
	}
	if err = send(c, 2); err != nil {
		cw.Stop()
		return err
	}
	if err = wait(ctx, "cloud writes replicated to source", func() (bool, error) {
		v, e := histories(s)
		for _, n := range v {
			if n == 12 {
				return true, e
			}
		}
		return false, e
	}); err != nil {
		cw.Stop()
		return err
	}
	if err = record("after cloud writes and replication"); err != nil {
		cw.Stop()
		return err
	}
	cw.Stop()
	if err = handover(c, "cluster-fvi-s", 2); err != nil {
		return err
	}
	if err = wait(ctx, "source reverse cutover", func() (bool, error) { r, e := getNS(ctx, s); return e == nil && r.GetFailoverVersion() == 29, e }); err != nil {
		return err
	}
	if err = record("after C to S handover before source writes"); err != nil {
		return err
	}
	sw, err = newWorker(s)
	if err != nil {
		return err
	}
	defer sw.Stop()
	if err = send(s, 3); err != nil {
		return err
	}
	if err = wait(ctx, "source writes replicated to cloud", func() (bool, error) {
		v, e := histories(c)
		for _, n := range v {
			if n == 29 {
				return true, e
			}
		}
		return false, e
	}); err != nil {
		return err
	}
	if err = record("after source writes and reverse replication"); err != nil {
		return err
	}
	if err = s.sdk.SignalWorkflow(ctx, id, "", "increment", -1); err != nil {
		return err
	}
	var result int
	if err = run.Get(ctx, &result); err != nil {
		return err
	}
	if result != 3 {
		return fmt.Errorf("unexpected workflow result %d", result)
	}
	if err = wait(ctx, "final histories equal", func() (bool, error) {
		a, e := readHistory(ctx, s, id)
		if e != nil {
			return false, e
		}
		b, e := readHistory(ctx, c, id)
		if e != nil {
			return false, e
		}
		return len(a.Events) > 0 && a.Events[len(a.Events)-1].GetEventType() == enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED && proto.Equal(a, b), nil
	}); err != nil {
		return err
	}
	if err = record("workflow completed with identical histories"); err != nil {
		return err
	}
	history, err := readHistory(ctx, s, id)
	if err != nil {
		return err
	}
	fmt.Printf("PASS workflow=%s run=%s result=%d events=%d histories_identical=true\n", id, run.GetRunID(), result, len(history.Events))
	return nil
}

func readHistory(ctx context.Context, e *endpoint, id string) (*historypb.History, error) {
	direct := metadata.AppendToOutgoingContext(ctx, "xdc-redirection", "false")
	it := e.sdk.GetWorkflowHistory(direct, id, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	result := &historypb.History{}
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			return nil, err
		}
		result.Events = append(result.Events, event)
	}
	return result, nil
}
