//go:build testcompatibility

package temporal_1_27_4_postgres12

import (
	_ "embed"

	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications"
)

//go:embed server_config.yaml
var serverConfigYAML string

//go:embed server_setup_schema.sh
var serverSetupSchemaTemplate string

func New() specifications.ClusterSpec {
	return specifications.ClusterSpec{
		Server: specifications.ServerSpec{
			Image:               "temporalio/server:1.27.4",
			AdminToolsImage:     "temporalio/admin-tools:1.27",
			ConfigTemplate:      serverConfigYAML,
			SetupSchemaTemplate: serverSetupSchemaTemplate,
		},
		Database: specifications.DatabaseSpec{
			Image: "postgres:12",
		},
	}
}
