package main

import (
	"go/format"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOutputMatchesCheckedIn runs the generator the way `make genvisitor` does
// and requires the result to match the checked-in file, so a dependency upgrade
// that changes the output (or breaks it) is caught here rather than by the next
// person to regenerate.
func TestOutputMatchesCheckedIn(t *testing.T) {
	out, err := exec.Command("go", "run", ".").Output()
	require.NoError(t, err)

	got, err := format.Source(out)
	require.NoError(t, err)

	want, err := os.ReadFile("../../../proto/compat/repair_utf8_gen.go")
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run make genvisitor")
}
