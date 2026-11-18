package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamespaceFailoverVersionRewriteValidation(t *testing.T) {
	valid := NamespaceFailoverVersionRule{Namespace: "migration", TargetCluster: "cloud", SourceInitialFailoverVersion: 1, SourceIncrement: 7, TargetInitialFailoverVersion: 2, TargetIncrement: 10, MinVersion: 9}
	require.NoError(t, (NamespaceFailoverVersionRewrite{}).Validate())
	require.NoError(t, (NamespaceFailoverVersionRewrite{Inbound: []NamespaceFailoverVersionRule{valid}}).Validate())
	require.Error(t, (NamespaceFailoverVersionRewrite{Inbound: []NamespaceFailoverVersionRule{valid, valid}}).Validate())
	for _, tc := range []struct {
		name   string
		mutate func(*NamespaceFailoverVersionRule)
	}{
		{"missing namespace", func(r *NamespaceFailoverVersionRule) { r.Namespace = "" }},
		{"zero increment", func(r *NamespaceFailoverVersionRule) { r.TargetIncrement = 0 }},
		{"initial version outside range", func(r *NamespaceFailoverVersionRule) { r.TargetInitialFailoverVersion = 10 }},
		{"same cluster identity", func(r *NamespaceFailoverVersionRule) { r.TargetInitialFailoverVersion = 1 }},
		{"missing epoch boundary", func(r *NamespaceFailoverVersionRule) { r.MinVersion = 0 }},
		{"oversized increment", func(r *NamespaceFailoverVersionRule) { r.SourceIncrement = 1 << 40 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := valid
			tc.mutate(&rule)
			require.Error(t, (NamespaceFailoverVersionRewrite{Outbound: []NamespaceFailoverVersionRule{rule}}).Validate())
		})
	}
}
