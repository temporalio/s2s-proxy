//go:build testcompatibility

package matrix

import (
	"testing"

	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications"
	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications/temporal_1_27_4_postgres12"
	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications/temporal_1_29_2_postgres15"
)

func TestTopology_1_29_2_same(t *testing.T) {
	t.Parallel()
	run(t, []specifications.ClusterSpec{
		temporal_1_29_2_postgres15.New(),
		temporal_1_29_2_postgres15.New(),
	})
}

func TestTopology_1_29_2_triplet(t *testing.T) {
	t.Parallel()
	run(t, []specifications.ClusterSpec{
		temporal_1_29_2_postgres15.New(),
		temporal_1_29_2_postgres15.New(),
		temporal_1_29_2_postgres15.New(),
	})
}

func TestTopology_1_29_2_vs_1_27_4(t *testing.T) {
	t.Parallel()
	run(t, []specifications.ClusterSpec{
		temporal_1_29_2_postgres15.New(),
		temporal_1_27_4_postgres12.New(),
	})
}
