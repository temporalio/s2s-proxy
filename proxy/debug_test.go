package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/failoverversion"
	"github.com/temporalio/s2s-proxy/interceptor"
)

func TestDebugInfoIncludesSafeFailoverVersionTranslationState(t *testing.T) {
	t.Parallel()
	descriptor := failoverversion.MappingDescriptor{
		SchemaVersion: failoverversion.MappingSchemaVersion,
		LocalFVI:      105,
		RemoteFVI:     100,
		LaneCount:     3,
		Fingerprint:   "sha256:test-fingerprint",
	}
	observer := interceptor.NewTranslationDebugObserver(2)
	observer.Observe(interceptor.TranslationObservation{
		Kind: "failover_version", Outcome: "error", ErrorCode: failoverversion.ErrorUnknownLane,
		Direction: failoverversion.LocalToRemote, Operation: "GetNamespaceReplicationMessagesRequest", FieldPath: "safe.path",
		NamespaceReplication: &interceptor.NamespaceReplicationObservation{LastRetrievedMessageID: 42},
	})
	instance := &Proxy{clusterConnections: map[migrationId]*ClusterConnection{
		{name: "z-pass-through"}: {},
		{name: "a-owner"}: {
			translationDescriptor: &descriptor,
			translationObserver:   observer,
		},
	}}

	recorder := httptest.NewRecorder()
	HandleDebugInfo(recorder, httptest.NewRequest("GET", "/debug/connections", nil), instance, log.NewTestLogger())
	require.Equal(t, 200, recorder.Code)

	var response DebugResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.FailoverVersionTranslations, 2)
	require.Positive(t, response.RuntimeMemory.HeapAlloc)
	require.Positive(t, response.RuntimeMemory.Sys)
	require.Positive(t, response.RuntimeMemory.Goroutines)
	owner := response.FailoverVersionTranslations[0]
	require.Equal(t, "a-owner", owner.ConnectionName)
	require.True(t, owner.Enabled)
	require.Equal(t, "owner", owner.Role)
	require.Equal(t, descriptor, owner.Mapping)
	require.Equal(t, failoverversion.LocalToRemote, owner.OutboundRequestDirection)
	require.Equal(t, failoverversion.RemoteToLocal, owner.OutboundResponseDirection)
	require.Equal(t, failoverversion.RemoteToLocal, owner.InboundRequestDirection)
	require.Equal(t, failoverversion.LocalToRemote, owner.InboundResponseDirection)
	require.Len(t, owner.Recent, 1)
	require.Len(t, owner.RecentFailures, 1)
	require.Equal(t, "safe.path", owner.RecentFailures[0].FieldPath)
	require.Len(t, owner.OperationSummaries, 1)
	require.Equal(t, "GetNamespaceReplicationMessagesRequest", owner.OperationSummaries[0].Operation)
	require.Equal(t, uint64(1), owner.OperationSummaries[0].Errors)
	require.Equal(t, int64(42), owner.Recent[0].NamespaceReplication.LastRetrievedMessageID)

	peer := response.FailoverVersionTranslations[1]
	require.False(t, peer.Enabled)
	require.Equal(t, "pass_through", peer.Role)
}
