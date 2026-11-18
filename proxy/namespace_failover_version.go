package proxy

import (
	"fmt"
	"math"

	replicationpb "go.temporal.io/api/replication/v1"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
)

// rewriteFailoverVersion preserves native target epochs and historical source
// epochs. Only source-generated target-owned epochs are mapped. The mapping is
// stateless, deterministic and idempotent; it never changes workflow payloads.
func rewriteFailoverVersion(v int64, r config.NamespaceFailoverVersionRule) (int64, error) {
	if err := (config.NamespaceFailoverVersionRewrite{Inbound: []config.NamespaceFailoverVersionRule{r}}).Validate(); err != nil {
		return 0, err
	}
	if v < r.MinVersion || v%r.TargetIncrement == r.TargetInitialFailoverVersion || v%r.SourceIncrement != r.TargetInitialFailoverVersion {
		return v, nil
	}
	delta := (r.TargetInitialFailoverVersion - v%r.TargetIncrement + r.TargetIncrement) % r.TargetIncrement
	if delta == 0 {
		delta = r.TargetIncrement
	}
	if v > math.MaxInt64-delta {
		return 0, fmt.Errorf("failover version overflow: %d", v)
	}
	next := v + delta
	if next%r.SourceIncrement == r.SourceInitialFailoverVersion {
		if r.TargetIncrement%r.SourceIncrement == 0 {
			return 0, fmt.Errorf("target epochs all identify source as local")
		}
		if next > math.MaxInt64-r.TargetIncrement {
			return 0, fmt.Errorf("failover version overflow: %d", next)
		}
		next += r.TargetIncrement
	}
	return next, nil
}

func (s *adminServiceProxyServer) rewriteNamespaceFailoverVersion(ns, active string, version *int64, history []*replicationpb.FailoverStatus) error {
	for _, rule := range s.overrides.NamespaceFailoverVersionRules {
		if rule.Namespace != ns || rule.TargetCluster != active {
			continue
		}
		before := *version
		after, err := rewriteFailoverVersion(before, rule)
		if err != nil {
			return err
		}
		// Prepare all changes before mutating the response.
		mapped := make([]int64, len(history))
		for i, item := range history {
			mapped[i], err = rewriteFailoverVersion(item.GetFailoverVersion(), rule)
			if err != nil {
				return err
			}
		}
		*version = after
		for i, item := range history {
			if item != nil {
				item.FailoverVersion = mapped[i]
			}
		}
		if before != after {
			s.loggers.Get(logging.AdminService).Info("Rewrote namespace handover failover version", tag.WorkflowNamespace(ns), tag.NewInt64("source-version", before), tag.NewInt64("target-version", after))
		}
	}
	return nil
}
