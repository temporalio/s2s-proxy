package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/failoverversion"
)

func TestGenerateFailoverVersionTranslation(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("bash generator")
	}
	dir := t.TempDir()
	fakeTemporal := filepath.Join(dir, "temporal")
	require.NoError(t, os.WriteFile(fakeTemporal, []byte(`#!/usr/bin/env bash
set -eu
args="$*"
if [[ "$args" == *"--profile source"*"operator cluster describe"* ]]; then
  printf '%s\n' '{"clusterName":"A","clusterId":"a-old","failoverVersionIncrement":"105","initialFailoverVersion":"1","isGlobalNamespaceEnabled":true}'
elif [[ "$args" == *"--profile target"*"operator cluster describe"* ]]; then
  printf '%s\n' '{"clusterName":"B","clusterId":"b-new","failoverVersionIncrement":"100","initialFailoverVersion":"2","isGlobalNamespaceEnabled":true}'
elif [[ "$args" == *"--profile source"*"operator cluster list"* ]]; then
  printf '%s\n' '[{"clusterName":"A","initialFailoverVersion":"1","clusterId":"a-old"},{"clusterName":"B","initialFailoverVersion":"2","clusterId":"b-old"}]'
elif [[ "$args" == *"--profile target"*"operator cluster list"* ]]; then
  printf '%s\n' '{"clusters":[{"clusterName":"B","initialFailoverVersion":"2","clusterId":"b-new"}]}'
elif [[ "$args" == *"--profile source"*"operator namespace describe"* ]]; then
  printf '%s\n' '{"namespaceInfo":{"id":"namespace-id"},"isGlobalNamespace":true,"replicationConfig":{"activeClusterName":"A","clusters":[{"clusterName":"A"}]},"failoverVersion":"211","failoverHistory":[{"failoverVersion":"107"}]}'
elif [[ "$args" == *"--profile target"*"operator namespace describe"* ]]; then
  printf 'Namespace Not Found\n' >&2
  exit 1
else
  printf 'unexpected invocation\n' >&2
  exit 2
fi
`), 0o700))

	script := filepath.Join("..", "generate-failover-version-translation.sh")
	cmd := exec.Command("bash", script,
		"--source-profile", "source",
		"--target-profile", "target",
		"--namespace", "migration",
		"--temporal-bin", fakeTemporal,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	require.NoError(t, err, stderr.String())
	require.NotEmpty(t, stdout, stderr.String())

	var generated struct {
		Translation struct {
			FVI struct {
				Local  int64 `yaml:"local"`
				Remote int64 `yaml:"remote"`
			} `yaml:"fvi"`
			IFV struct {
				Mappings []struct {
					Local  int64 `yaml:"local"`
					Remote int64 `yaml:"remote"`
				} `yaml:"mappings"`
			} `yaml:"ifv"`
		} `yaml:"failoverVersionTranslation"`
	}
	require.NoError(t, yaml.Unmarshal(stdout, &generated))
	require.Equal(t, int64(105), generated.Translation.FVI.Local)
	require.Equal(t, int64(100), generated.Translation.FVI.Remote)
	require.Equal(t, int64(1), generated.Translation.IFV.Mappings[0].Local)
	require.Equal(t, int64(1), generated.Translation.IFV.Mappings[0].Remote)
	require.Equal(t, int64(2), generated.Translation.IFV.Mappings[1].Local)
	require.Equal(t, int64(2), generated.Translation.IFV.Mappings[1].Remote)

	mappingConfig := config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: generated.Translation.FVI.Local, Remote: generated.Translation.FVI.Remote},
	}
	for _, pair := range generated.Translation.IFV.Mappings {
		mappingConfig.IFV.Mappings = append(mappingConfig.IFV.Mappings, config.IntMapping{Local: pair.Local, Remote: pair.Remote})
	}
	mapping, err := failoverversion.NewMapping(mappingConfig)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "mapping-fingerprint: "+mapping.Descriptor().Fingerprint)
}

func TestGenerateFailoverVersionTranslationWithoutClusterGlobalFlag(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("bash generator")
	}
	dir := t.TempDir()
	fakeTemporal := filepath.Join(dir, "temporal")
	require.NoError(t, os.WriteFile(fakeTemporal, []byte(`#!/usr/bin/env bash
set -eu
args="$*"
if [[ "$args" == *"--profile source"*"operator cluster describe"* ]]; then
  printf '%s\n' '{"clusterName":"A","clusterId":"a","failoverVersionIncrement":"105","initialFailoverVersion":"1"}'
elif [[ "$args" == *"--profile target"*"operator cluster describe"* ]]; then
  printf '%s\n' '{"clusterName":"B","clusterId":"b","failoverVersionIncrement":"100","initialFailoverVersion":"2"}'
elif [[ "$args" == *"operator cluster list"* ]]; then
  if [[ "$args" == *"--profile source"* ]]; then
    printf '%s\n' '{"clusters":[{"clusterName":"A","initialFailoverVersion":"1","clusterId":"a"}]}'
  else
    printf '%s\n' '{"clusters":[{"clusterName":"B","initialFailoverVersion":"2","clusterId":"b"}]}'
  fi
elif [[ "$args" == *"--profile source"*"operator namespace describe"* ]]; then
  printf '%s\n' '{"namespaceInfo":{"id":"namespace-id"},"isGlobalNamespace":true,"replicationConfig":{"activeClusterName":"A","clusters":[{"clusterName":"A"}]},"failoverVersion":"1"}'
elif [[ "$args" == *"--profile target"*"operator namespace describe"* ]]; then
  printf 'Namespace Not Found\n' >&2
  exit 1
else
  exit 2
fi
`), 0o700))

	script := filepath.Join("..", "generate-failover-version-translation.sh")
	cmd := exec.Command("bash", script,
		"--source-profile", "source",
		"--target-profile", "target",
		"--namespace", "migration",
		"--temporal-bin", fakeTemporal,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	require.NoError(t, err, stderr.String())
	require.Contains(t, stderr.String(), "does not expose the global-namespace setting")
	require.Contains(t, string(stdout), "failoverVersionTranslation:")
}
