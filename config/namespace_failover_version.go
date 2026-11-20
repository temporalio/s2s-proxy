package config

import (
	"errors"
	"fmt"
	"math"
)

type NamespaceFailoverVersionRewriteMode string

const (
	NamespaceFailoverVersionRewriteModeObserve NamespaceFailoverVersionRewriteMode = "observe"
	NamespaceFailoverVersionRewriteModeEnforce NamespaceFailoverVersionRewriteMode = "enforce"
)

// NamespaceFailoverVersionRewriteConfig enables the experimental two-cluster
// namespace cutover rewrite. Namespaces use their local names; the proxy uses
// namespaceTranslation to derive their remote names. Native FVIs come from the
// crossed failoverVersionIncrementTranslation values on the connection.
type NamespaceFailoverVersionRewriteConfig struct {
	Mode       NamespaceFailoverVersionRewriteMode `yaml:"mode"`
	Namespaces []string                            `yaml:"namespaces"`
	Local      NamespaceFailoverVersionEndpoint    `yaml:"local"`
	Remote     NamespaceFailoverVersionEndpoint    `yaml:"remote"`
}

type NamespaceFailoverVersionEndpoint struct {
	ClusterName            string `yaml:"clusterName"`
	InitialFailoverVersion int64  `yaml:"initialFailoverVersion"`
}

func (c *NamespaceFailoverVersionRewriteConfig) Validate(fviTranslation IntMapping) error {
	if c == nil {
		return nil
	}
	if c.Mode != NamespaceFailoverVersionRewriteModeObserve && c.Mode != NamespaceFailoverVersionRewriteModeEnforce {
		return fmt.Errorf("mode must be %q or %q", NamespaceFailoverVersionRewriteModeObserve, NamespaceFailoverVersionRewriteModeEnforce)
	}
	if len(c.Namespaces) == 0 {
		return errors.New("namespaces must contain at least one local namespace name")
	}
	seen := make(map[string]struct{}, len(c.Namespaces))
	for _, namespace := range c.Namespaces {
		if namespace == "" {
			return errors.New("namespaces must not contain an empty name")
		}
		if _, exists := seen[namespace]; exists {
			return fmt.Errorf("namespaces contains duplicate %q", namespace)
		}
		seen[namespace] = struct{}{}
	}
	if c.Local.ClusterName == "" || c.Remote.ClusterName == "" {
		return errors.New("local.clusterName and remote.clusterName are required")
	}
	if c.Local.ClusterName == c.Remote.ClusterName {
		return errors.New("local.clusterName and remote.clusterName must differ")
	}
	localFVI, remoteFVI := fviTranslation.Remote, fviTranslation.Local
	if localFVI <= 0 || localFVI > math.MaxInt32 || remoteFVI <= 0 || remoteFVI > math.MaxInt32 {
		return fmt.Errorf("failoverVersionIncrementTranslation values must be in 1..%d", int64(math.MaxInt32))
	}
	if localFVI == remoteFVI {
		return errors.New("local and remote failoverVersionIncrement must differ")
	}
	if c.Local.InitialFailoverVersion == c.Remote.InitialFailoverVersion {
		return errors.New("local and remote initialFailoverVersion must differ")
	}
	minimumFVI := min(localFVI, remoteFVI)
	if c.Local.InitialFailoverVersion <= 0 || c.Remote.InitialFailoverVersion <= 0 ||
		c.Local.InitialFailoverVersion > minimumFVI || c.Remote.InitialFailoverVersion > minimumFVI {
		return fmt.Errorf("both initialFailoverVersion values must fit in both FVI domains (maximum %d)", minimumFVI)
	}
	if !namespaceRewriteCandidateExists(localFVI, c.Local.InitialFailoverVersion, remoteFVI, c.Remote.InitialFailoverVersion) {
		return errors.New("local-to-remote has no receiver-owned candidate on an unallocated sender lane")
	}
	if !namespaceRewriteCandidateExists(remoteFVI, c.Remote.InitialFailoverVersion, localFVI, c.Local.InitialFailoverVersion) {
		return errors.New("remote-to-local has no receiver-owned candidate on an unallocated sender lane")
	}
	return nil
}

// With two allocated sender lanes, one of the first three distinct receiver
// residues must be free. If the residues repeat sooner, no free lane exists.
func namespaceRewriteCandidateExists(senderFVI, senderIFV, receiverFVI, receiverIFV int64) bool {
	candidate := receiverIFV
	seen := make(map[int64]struct{}, 3)
	for range 3 {
		candidateLane := namespaceRewriteLane(candidate, senderFVI)
		if _, exists := seen[candidateLane]; exists {
			return false
		}
		if candidateLane != senderIFV && candidateLane != receiverIFV {
			return true
		}
		seen[candidateLane] = struct{}{}
		candidate += receiverFVI
	}
	return false
}

func namespaceRewriteLane(version, fvi int64) int64 {
	return ((version - 1) % fvi) + 1
}
