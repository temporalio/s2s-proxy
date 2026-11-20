package metrics

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

func TestClientTransportWaitStatsHandler(t *testing.T) {
	tests := []struct {
		name        string
		pool        string
		method      string
		begin       stats.Begin
		endErr      error
		outHeader   bool
		wantPool    string
		wantType    string
		wantService string
		wantMethod  string
		wantOutcome string
	}{
		{
			name:        "unary becomes ready",
			pool:        "general",
			method:      "/temporal.api.workflowservice.v1.WorkflowService/DescribeWorkflowExecution",
			outHeader:   true,
			wantPool:    "general",
			wantType:    "unary",
			wantService: "temporal.api.workflowservice.v1.WorkflowService",
			wantMethod:  "DescribeWorkflowExecution",
			wantOutcome: "ready",
		},
		{
			name:        "client stream exceeds deadline",
			pool:        "replication",
			method:      "/temporal.api.adminservice.v1.AdminService/GetReplicationMessages",
			begin:       stats.Begin{IsClientStream: true},
			endErr:      status.Error(codes.DeadlineExceeded, "deadline"),
			wantPool:    "replication",
			wantType:    "client_stream",
			wantService: "temporal.api.adminservice.v1.AdminService",
			wantMethod:  "GetReplicationMessages",
			wantOutcome: "deadline_exceeded_before_header",
		},
		{
			name:        "server stream is canceled",
			method:      "/temporal.api.adminservice.v1.AdminService/StreamWorkflowReplicationMessages",
			begin:       stats.Begin{IsServerStream: true},
			endErr:      status.Error(codes.Canceled, "canceled"),
			wantPool:    "shared",
			wantType:    "server_stream",
			wantService: "temporal.api.adminservice.v1.AdminService",
			wantMethod:  "StreamWorkflowReplicationMessages",
			wantOutcome: "canceled_before_header",
		},
		{
			name:        "bidi stream ends before headers",
			pool:        "replication",
			method:      "Malformed",
			begin:       stats.Begin{IsClientStream: true, IsServerStream: true},
			wantPool:    "replication",
			wantType:    "bidi_stream",
			wantService: "unknown",
			wantMethod:  "Malformed",
			wantOutcome: "ended_before_header",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			direction := fmt.Sprintf("transport-wait-test-%d", i)
			handler := NewClientTransportWaitStatsHandler(direction, tt.pool)
			ctx := handler.TagRPC(context.Background(), &stats.RPCTagInfo{FullMethodName: tt.method})
			histogramLabels := map[string]string{
				"direction":    direction,
				"pool":         tt.wantPool,
				"grpc_type":    tt.wantType,
				"grpc_service": tt.wantService,
				"grpc_method":  tt.wantMethod,
				"outcome":      tt.wantOutcome,
			}
			before := histogramCountOrZero(t, "temporal_s2s_proxy_grpc_client_time_to_out_header_seconds", histogramLabels)

			handler.HandleRPC(ctx, &tt.begin)
			handler.HandleRPC(ctx, &tt.begin) // Duplicate begin events must not increment twice.
			require.Equal(t, float64(1), gaugeValue(t, "temporal_s2s_proxy_grpc_client_transport_waiting", map[string]string{
				"direction": direction,
				"pool":      tt.wantPool,
			}))

			if tt.outHeader {
				handler.HandleRPC(ctx, &stats.OutHeader{})
				handler.HandleRPC(ctx, &stats.End{}) // A later terminal event must not record twice.
			} else {
				handler.HandleRPC(ctx, &stats.End{Error: tt.endErr})
				handler.HandleRPC(ctx, &stats.OutHeader{}) // A late header must not record twice.
			}

			require.Equal(t, float64(0), gaugeValue(t, "temporal_s2s_proxy_grpc_client_transport_waiting", map[string]string{
				"direction": direction,
				"pool":      tt.wantPool,
			}))
			require.Equal(t, before+1, histogramCount(t, "temporal_s2s_proxy_grpc_client_time_to_out_header_seconds", histogramLabels))
		})
	}
}

func TestSplitFullMethod(t *testing.T) {
	for _, tt := range []struct {
		name        string
		fullMethod  string
		wantService string
		wantMethod  string
	}{
		{name: "canonical", fullMethod: "/package.Service/Method", wantService: "package.Service", wantMethod: "Method"},
		{name: "without leading slash", fullMethod: "package.Service/Method", wantService: "package.Service", wantMethod: "Method"},
		{name: "malformed", fullMethod: "Malformed", wantService: "unknown", wantMethod: "Malformed"},
		{name: "empty", wantService: "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, method := splitFullMethod(tt.fullMethod)
			require.Equal(t, tt.wantService, service)
			require.Equal(t, tt.wantMethod, method)
		})
	}
}

func gaugeValue(t *testing.T, metricName string, labels map[string]string) float64 {
	t.Helper()
	metric := findMetric(t, metricName, labels)
	require.NotNil(t, metric.Gauge)
	return metric.Gauge.GetValue()
}

func histogramCount(t *testing.T, metricName string, labels map[string]string) uint64 {
	t.Helper()
	metric := findMetric(t, metricName, labels)
	require.NotNil(t, metric.Histogram)
	return metric.Histogram.GetSampleCount()
}

func histogramCountOrZero(t *testing.T, metricName string, labels map[string]string) uint64 {
	t.Helper()
	metric := lookupMetric(t, metricName, labels)
	if metric == nil {
		return 0
	}
	require.NotNil(t, metric.Histogram)
	return metric.Histogram.GetSampleCount()
}

func findMetric(t *testing.T, metricName string, labels map[string]string) *dto.Metric {
	t.Helper()
	metric := lookupMetric(t, metricName, labels)
	if metric == nil {
		t.Fatalf("metric %s with labels %v not found", metricName, labels)
	}
	return metric
}

func lookupMetric(t *testing.T, metricName string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.Metric {
			actual := make(map[string]string, len(metric.Label))
			for _, label := range metric.Label {
				actual[label.GetName()] = label.GetValue()
			}
			if reflect.DeepEqual(labels, actual) {
				return metric
			}
		}
	}
	return nil
}
