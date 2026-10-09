package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
)

func TestGenerateUTF8MatchesCheckedIn(t *testing.T) {
	got, err := generate(log.NewNoopLogger(), "utf8", nil)
	require.NoError(t, err)

	want, err := os.ReadFile("../../../proto/compat/repair_utf8_gen.go")
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run make genvisitor")
}

func TestGenerateUnknownTarget(t *testing.T) {
	_, err := generate(log.NewNoopLogger(), "nope", nil)
	require.ErrorContains(t, err, `unknown -target "nope"`)
}
