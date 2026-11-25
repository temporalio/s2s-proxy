package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// The CHASM schema is read from the server's registry, so a server upgrade that
// adds or changes a component, data node, or task fails here until it is
// regenerated.
func TestGenerateMatchesCheckedIn(t *testing.T) {
	got, err := generate()
	require.NoError(t, err)

	want, err := os.ReadFile("../../../interceptor/chasm_schema_gen.go")
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run make genvisitor-admin-payloads")
}
