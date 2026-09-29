package preflight

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportWriteText(t *testing.T) {
	report := Report{
		ConfigPath:   "/config/config.yaml",
		ConfigSHA256: "abc123",
		Version:      "v-test",
		Pod:          "proxy-pod-0",
		Scope:        GenericProxyScope,
		Results: []Result{
			{Status: StatusPass, Name: "good"},
			{Status: StatusWarn, Name: "concern", Details: []string{"review this value"}},
			{Status: StatusFail, Name: "bad"},
			{Status: StatusSkip, Name: "not applicable"},
			{Status: StatusUnknown, Name: "not observed"},
			{Status: StatusInfo, Name: "context"},
		},
	}

	var output bytes.Buffer
	require.NoError(t, report.WriteText(&output))

	assert.Contains(t, output.String(), "config  /config/config.yaml  sha256 abc123")
	assert.Contains(t, output.String(), "WARN    concern")
	assert.Contains(t, output.String(), "review this value")
	assert.Contains(t, output.String(), "1 failed, 1 warnings, 1 passed, 1 skipped, 1 unknown")
	assert.Contains(t, output.String(), "scope: "+GenericProxyScope)
	assert.True(t, report.HasFailures())
	assert.True(t, report.HasUnknowns())
}

func TestReportWriteJSON(t *testing.T) {
	report := Report{
		ConfigPath:   "/config/config.yaml",
		ConfigSHA256: "abc123",
		Version:      "v-test",
		Pod:          "proxy-pod-0",
		Scope:        GenericProxyScope,
		Results:      []Result{{Status: StatusPass, Name: "good"}},
	}

	var output bytes.Buffer
	require.NoError(t, report.WriteJSON(&output))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &decoded))
	assert.Equal(t, "/config/config.yaml", decoded["configPath"])
	assert.Equal(t, "abc123", decoded["configSHA256"])
	assert.Equal(t, GenericProxyScope, decoded["scope"])
}
