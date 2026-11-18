package config

import (
	"fmt"
	"math"
)

// NamespaceFailoverVersionRewrite is an opt-in migration prototype. Names are
// matched as returned by the upstream cluster, before response translation.
type NamespaceFailoverVersionRewrite struct {
	Inbound  []NamespaceFailoverVersionRule `yaml:"inbound"`
	Outbound []NamespaceFailoverVersionRule `yaml:"outbound"`
}

type NamespaceFailoverVersionRule struct {
	Namespace                    string `yaml:"namespace"`
	TargetCluster                string `yaml:"targetCluster"`
	SourceInitialFailoverVersion int64  `yaml:"sourceInitialFailoverVersion"`
	SourceIncrement              int64  `yaml:"sourceIncrement"`
	TargetInitialFailoverVersion int64  `yaml:"targetInitialFailoverVersion"`
	TargetIncrement              int64  `yaml:"targetIncrement"`
	// MinVersion excludes epochs before this migration attempt.
	MinVersion int64 `yaml:"minVersion"`
}

func (c NamespaceFailoverVersionRewrite) Validate() error {
	for direction, rules := range map[string][]NamespaceFailoverVersionRule{"inbound": c.Inbound, "outbound": c.Outbound} {
		seen := map[string]bool{}
		for _, r := range rules {
			if r.Namespace == "" || r.TargetCluster == "" || r.SourceIncrement <= 0 || r.TargetIncrement <= 0 || r.SourceIncrement > math.MaxInt32 || r.TargetIncrement > math.MaxInt32 || r.SourceInitialFailoverVersion <= 0 || r.SourceInitialFailoverVersion >= r.SourceIncrement || r.TargetInitialFailoverVersion <= 0 || r.TargetInitialFailoverVersion >= r.TargetIncrement || r.TargetInitialFailoverVersion >= r.SourceIncrement || r.MinVersion <= 0 {
				return fmt.Errorf("%s: invalid namespace failover version rule for %q", direction, r.Namespace)
			}
			key := r.Namespace + "/" + r.TargetCluster
			if seen[key] {
				return fmt.Errorf("%s: duplicate namespace failover version rule %q", direction, key)
			}
			seen[key] = true
			if r.SourceInitialFailoverVersion == r.TargetInitialFailoverVersion {
				return fmt.Errorf("%s: source and target initial versions must differ", direction)
			}
		}
	}
	return nil
}
