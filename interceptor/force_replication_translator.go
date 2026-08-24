package interceptor

import (
	"encoding/json"
	"fmt"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
)

const (
	// Registered by the server's service/worker/migration package.
	forceReplicationWorkflowType = "force-replication"

	// ForceReplicationParams carries no json tags, so the key is the Go field name verbatim.
	targetClusterEndpointKey = "TargetClusterEndpoint"

	payloadEncodingMetadataKey = "encoding"
	jsonPlainEncoding          = "json/plain"

	// WorkflowServicePrefix already ends with "/".
	startWorkflowExecutionMethod = api.WorkflowServicePrefix + "StartWorkflowExecution"
)

type (
	// frEndpointTranslator rewrites ForceReplicationParams.TargetClusterEndpoint in the
	// StartWorkflowExecution request that kicks off the force replication workflow.
	//
	// Temporal servers older than v1.22.2 dial that address verbatim from the
	// VerifyReplicationTasks activity, and the address the caller sends is the remote cluster's own
	// address, which is generally not routable from the local cluster. Rewriting it to this proxy's
	// replicationEndpoint is the same correction the proxy already applies to FrontendAddress in
	// AddOrUpdateRemoteCluster; that one is a typed proto field, this one rides inside a
	// workflow-args payload. TargetClusterName is left alone, so servers from v1.22.2 on (which
	// prefer the name and resolve the address from their own cluster registry) are unaffected.
	//
	// Unlike the other translators this one does not use the reflection visitor: it type-asserts the
	// request instead, which is also what keeps it inert on the stream path where MatchMethod is
	// never consulted and every translator sees every message.
	frEndpointTranslator struct {
		logger              log.Logger
		replicationEndpoint string
	}
)

func NewForceReplicationEndpointTranslator(logger log.Logger, replicationEndpoint string) Translator {
	return &frEndpointTranslator{
		logger:              logger,
		replicationEndpoint: replicationEndpoint,
	}
}

func (t *frEndpointTranslator) Kind() string {
	return metrics.ForceReplicationEndpointTranslationKind
}

func (t *frEndpointTranslator) MatchMethod(m string) bool {
	return m == startWorkflowExecutionMethod
}

func (t *frEndpointTranslator) TranslateRequest(req any) (bool, error) {
	// Type assert first: on the stream path MatchMethod is not consulted, so this is what keeps the
	// translator inert there.
	r, ok := req.(*workflowservice.StartWorkflowExecutionRequest)
	if !ok || r.GetWorkflowType().GetName() != forceReplicationWorkflowType {
		return false, nil
	}

	// Past this point the request is definitely a force replication start. Any failure to rewrite is
	// reported as an error so it shows up on the translation_error metric under this Kind: a silent
	// no-op here means a migration that hangs for a week with no signal.
	payloads := r.GetInput().GetPayloads()
	if len(payloads) == 0 {
		return false, fmt.Errorf("%s request has no input payloads", forceReplicationWorkflowType)
	}

	payload := payloads[0]
	if encoding := string(payload.GetMetadata()[payloadEncodingMetadataKey]); encoding != jsonPlainEncoding {
		return false, fmt.Errorf("%s params have unsupported payload encoding %q, want %q",
			forceReplicationWorkflowType, encoding, jsonPlainEncoding)
	}

	var params map[string]json.RawMessage
	if err := json.Unmarshal(payload.GetData(), &params); err != nil {
		return false, fmt.Errorf("failed to decode %s params: %w", forceReplicationWorkflowType, err)
	}
	if _, found := params[targetClusterEndpointKey]; !found {
		return false, fmt.Errorf("%s params have no %s field", forceReplicationWorkflowType, targetClusterEndpointKey)
	}

	newEndpoint, err := json.Marshal(t.replicationEndpoint)
	if err != nil {
		return false, fmt.Errorf("failed to encode %s: %w", targetClusterEndpointKey, err)
	}
	params[targetClusterEndpointKey] = newEndpoint

	data, err := json.Marshal(params)
	if err != nil {
		return false, fmt.Errorf("failed to re-encode %s params: %w", forceReplicationWorkflowType, err)
	}

	// Mutate in place so the payload's Metadata is preserved by construction.
	payload.Data = data
	t.logger.Info("Overwrote force replication target cluster endpoint",
		tag.Address(t.replicationEndpoint))
	return true, nil
}

func (t *frEndpointTranslator) TranslateResponse(any) (bool, error) {
	return false, nil
}
