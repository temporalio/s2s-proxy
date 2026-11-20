package proxy

import (
	"fmt"
	"math"

	replicationpb "go.temporal.io/api/replication/v1"
	adminservicepb "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	namespaceRewriteEndpoint struct {
		clusterName string
		fvi         int64
		ifv         int64
	}

	NamespaceFailoverVersionRewriter struct {
		mode       config.NamespaceFailoverVersionRewriteMode
		connection string
		direction  string
		sender     namespaceRewriteEndpoint
		receiver   namespaceRewriteEndpoint
		// source namespace name -> canonical local namespace name
		namespaces map[string]string
	}

	namespaceRewriteObservation struct {
		namespace string
		operation string
		result    string
		reason    string
		input     int64
		output    int64
	}

	namespaceRewritePlan struct {
		version          *int64
		history          []*replicationpb.FailoverStatus
		output           int64
		observationIndex int
	}
)

const (
	namespaceRewriteDirectionLocalToRemote = "local_to_remote"
	namespaceRewriteDirectionRemoteToLocal = "remote_to_local"

	namespaceRewriteOperationTask         = "namespace_task"
	namespaceRewriteOperationGetNamespace = "get_namespace"

	namespaceRewriteResultOutOfScope   = "out_of_scope"
	namespaceRewriteResultInactive     = "inactive"
	namespaceRewriteResultNotCutover   = "not_cutover"
	namespaceRewriteResultWouldRewrite = "would_rewrite"
	namespaceRewriteResultRewritten    = "rewritten"
	namespaceRewriteResultError        = "error"
)

func newNamespaceFailoverVersionRewriters(
	connection string,
	cfg *config.NamespaceFailoverVersionRewriteConfig,
	fviTranslation config.IntMapping,
	namespaceTranslation collect.StaticBiMap[string, string],
) (localToRemote, remoteToLocal *NamespaceFailoverVersionRewriter, err error) {
	if cfg == nil {
		return nil, nil, nil
	}
	if err := cfg.Validate(fviTranslation); err != nil {
		return nil, nil, err
	}

	localNamespaces := make(map[string]string, len(cfg.Namespaces))
	remoteNamespaces := make(map[string]string, len(cfg.Namespaces))
	for _, localName := range cfg.Namespaces {
		remoteName := localName
		if translated, exists := namespaceTranslation.GetExists(localName); exists {
			remoteName = translated
		}
		if canonical, exists := remoteNamespaces[remoteName]; exists && canonical != localName {
			return nil, nil, fmt.Errorf("namespace allowlist maps %q and %q to remote name %q", canonical, localName, remoteName)
		}
		localNamespaces[localName] = localName
		remoteNamespaces[remoteName] = localName
	}

	local := namespaceRewriteEndpoint{
		clusterName: cfg.Local.ClusterName,
		fvi:         fviTranslation.Remote,
		ifv:         cfg.Local.InitialFailoverVersion,
	}
	remote := namespaceRewriteEndpoint{
		clusterName: cfg.Remote.ClusterName,
		fvi:         fviTranslation.Local,
		ifv:         cfg.Remote.InitialFailoverVersion,
	}
	return &NamespaceFailoverVersionRewriter{
		mode:       cfg.Mode,
		connection: connection,
		direction:  namespaceRewriteDirectionLocalToRemote,
		sender:     local,
		receiver:   remote,
		namespaces: localNamespaces,
	}, &NamespaceFailoverVersionRewriter{
		mode:       cfg.Mode,
		connection: connection,
		direction:  namespaceRewriteDirectionRemoteToLocal,
		sender:     remote,
		receiver:   local,
		namespaces: remoteNamespaces,
	}, nil
}

func (r *NamespaceFailoverVersionRewriter) rewriteGetNamespace(
	response *adminservicepb.GetNamespaceResponse,
) ([]namespaceRewriteObservation, error) {
	if r == nil || response == nil {
		return nil, nil
	}
	plan, observation, err := r.plan(
		namespaceRewriteOperationGetNamespace,
		response.GetInfo().GetName(),
		response.GetReplicationConfig().GetActiveClusterName(),
		response.GetReplicationConfig().GetClusters(),
		&response.FailoverVersion,
		response.GetFailoverHistory(),
	)
	if err != nil {
		if r.mode == config.NamespaceFailoverVersionRewriteModeObserve {
			return []namespaceRewriteObservation{observation}, nil
		}
		return []namespaceRewriteObservation{observation}, err
	}
	if plan == nil {
		return []namespaceRewriteObservation{observation}, nil
	}
	if r.mode == config.NamespaceFailoverVersionRewriteModeObserve {
		observation.result = namespaceRewriteResultWouldRewrite
	} else {
		applyNamespaceRewrite(*plan)
		observation.result = namespaceRewriteResultRewritten
	}
	return []namespaceRewriteObservation{observation}, nil
}

func (r *NamespaceFailoverVersionRewriter) rewriteNamespaceReplicationMessages(
	response *adminservicepb.GetNamespaceReplicationMessagesResponse,
) ([]namespaceRewriteObservation, error) {
	if r == nil || response == nil {
		return nil, nil
	}

	var observations []namespaceRewriteObservation
	var plans []namespaceRewritePlan
	for _, task := range response.GetMessages().GetReplicationTasks() {
		attributes := task.GetNamespaceTaskAttributes()
		if attributes == nil {
			continue
		}
		plan, observation, err := r.plan(
			namespaceRewriteOperationTask,
			attributes.GetInfo().GetName(),
			attributes.GetReplicationConfig().GetActiveClusterName(),
			attributes.GetReplicationConfig().GetClusters(),
			&attributes.FailoverVersion,
			attributes.GetFailoverHistory(),
		)
		observations = append(observations, observation)
		if err != nil {
			if r.mode == config.NamespaceFailoverVersionRewriteModeEnforce {
				return observations, err
			}
			continue
		}
		if plan != nil {
			plan.observationIndex = len(observations) - 1
			plans = append(plans, *plan)
		}
	}

	for _, plan := range plans {
		if r.mode == config.NamespaceFailoverVersionRewriteModeObserve {
			observations[plan.observationIndex].result = namespaceRewriteResultWouldRewrite
			continue
		}
		applyNamespaceRewrite(plan)
		observations[plan.observationIndex].result = namespaceRewriteResultRewritten
	}
	return observations, nil
}

func (r *NamespaceFailoverVersionRewriter) plan(
	operation, namespace, activeCluster string,
	clusters []*replicationpb.ClusterReplicationConfig,
	version *int64,
	history []*replicationpb.FailoverStatus,
) (*namespaceRewritePlan, namespaceRewriteObservation, error) {
	canonicalNamespace, allowed := r.namespaces[namespace]
	observation := namespaceRewriteObservation{namespace: canonicalNamespace, operation: operation}
	if !allowed {
		observation.result = namespaceRewriteResultOutOfScope
		return nil, observation, nil
	}
	observation.input = *version

	members := make(map[string]struct{}, len(clusters))
	for _, cluster := range clusters {
		if name := cluster.GetClusterName(); name != "" {
			members[name] = struct{}{}
		}
	}
	_, hasSender := members[r.sender.clusterName]
	_, hasReceiver := members[r.receiver.clusterName]
	if !hasSender || !hasReceiver {
		observation.result = namespaceRewriteResultOutOfScope
		return nil, observation, nil
	}
	if len(members) != 2 || (activeCluster != r.sender.clusterName && activeCluster != r.receiver.clusterName) {
		return r.errorObservation(observation, "unsupported_topology")
	}
	if activeCluster == r.sender.clusterName {
		observation.result = namespaceRewriteResultInactive
		return nil, observation, nil
	}
	if *version <= 0 {
		return r.errorObservation(observation, "invalid_version")
	}
	if namespaceRewriteLane(*version, r.sender.fvi) != r.receiver.ifv {
		observation.result = namespaceRewriteResultNotCutover
		return nil, observation, nil
	}

	output, reason := r.nextSafeVersion(*version)
	if reason != "" {
		return r.errorObservation(observation, reason)
	}
	if len(history) == 0 || history[len(history)-1] == nil {
		return r.errorObservation(observation, "missing_history")
	}
	if history[len(history)-1].GetFailoverVersion() != *version {
		return r.errorObservation(observation, "history_mismatch")
	}
	observation.output = output
	return &namespaceRewritePlan{version: version, history: history, output: output}, observation, nil
}

func (r *NamespaceFailoverVersionRewriter) nextSafeVersion(after int64) (int64, string) {
	candidate, ok := namespaceRewriteNextVersion(after, r.receiver.fvi, r.receiver.ifv)
	if !ok {
		return 0, "overflow"
	}
	seen := make(map[int64]struct{}, 3)
	for range 3 {
		candidateLane := namespaceRewriteLane(candidate, r.sender.fvi)
		if _, exists := seen[candidateLane]; exists {
			return 0, "no_candidate"
		}
		if candidateLane != r.sender.ifv && candidateLane != r.receiver.ifv {
			return candidate, ""
		}
		seen[candidateLane] = struct{}{}
		if candidate > math.MaxInt64-r.receiver.fvi {
			return 0, "overflow"
		}
		candidate += r.receiver.fvi
	}
	return 0, "no_candidate"
}

func (r *NamespaceFailoverVersionRewriter) errorObservation(
	observation namespaceRewriteObservation,
	reason string,
) (*namespaceRewritePlan, namespaceRewriteObservation, error) {
	observation.result = namespaceRewriteResultError
	observation.reason = reason
	return nil, observation, fmt.Errorf("namespace failover version rewrite failed: %s", reason)
}

func applyNamespaceRewrite(plan namespaceRewritePlan) {
	*plan.version = plan.output
	plan.history[len(plan.history)-1].FailoverVersion = plan.output
}

func namespaceRewriteLane(version, fvi int64) int64 {
	return ((version - 1) % fvi) + 1
}

func namespaceRewriteNextVersion(after, fvi, ifv int64) (int64, bool) {
	epoch := (after - 1) / fvi
	if epoch > (math.MaxInt64-ifv)/fvi {
		return 0, false
	}
	candidate := epoch*fvi + ifv
	if candidate > after {
		return candidate, true
	}
	if candidate > math.MaxInt64-fvi {
		return 0, false
	}
	return candidate + fvi, true
}

func (s *adminServiceProxyServer) recordNamespaceRewriteObservations(observations []namespaceRewriteObservation) {
	for _, observation := range observations {
		if observation.result == "" {
			continue
		}
		metrics.NamespaceFailoverVersionRewrite.WithLabelValues(
			s.overrides.NamespaceFailoverVersionRewriter.connection,
			s.overrides.NamespaceFailoverVersionRewriter.direction,
			observation.operation,
			observation.result,
		).Inc()
		if observation.result != namespaceRewriteResultWouldRewrite &&
			observation.result != namespaceRewriteResultRewritten &&
			observation.result != namespaceRewriteResultError {
			continue
		}
		logger := s.loggers.Get(logging.AdminService)
		tags := []tag.Tag{
			tag.WorkflowNamespace(observation.namespace),
			tag.NewStringTag("direction", s.overrides.NamespaceFailoverVersionRewriter.direction),
			tag.Operation(observation.operation),
			tag.NewStringTag("result", observation.result),
			tag.NewInt64("input-version", observation.input),
		}
		if observation.output != 0 {
			tags = append(tags, tag.NewInt64("output-version", observation.output))
		}
		if observation.reason != "" {
			tags = append(tags, tag.NewStringTag("reason", observation.reason))
		}
		if observation.result == namespaceRewriteResultError {
			logger.Error("Namespace failover version rewrite failed", tags...)
		} else {
			logger.Info("Namespace failover version rewrite", tags...)
		}
	}
}
