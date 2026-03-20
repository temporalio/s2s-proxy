//go:build testcompatibility

package temporal_1_29_2_postgres15

import (
	_ "embed"

	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications"
)

//go:embed server_config.yaml
var serverConfigTemplate string

//go:embed server_setup_schema.sh
var setupSchemaTemplate string

func New() specifications.ClusterSpec {
	return specifications.ClusterSpec{
		Server: specifications.ServerSpec{
			Image:               "temporalio/server:1.29.2",
			AdminToolsImage:     "temporalio/admin-tools:1.29",
			ConfigTemplate:      serverConfigTemplate,
			SetupSchemaTemplate: setupSchemaTemplate,
		},
		Database: specifications.DatabaseSpec{
			Image: "postgres:15",
		},
	}
}
