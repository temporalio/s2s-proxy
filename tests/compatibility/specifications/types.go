//go:build testcompatibility

package specifications

// ClusterSpec describes the Temporal Cluster configuration.
type ClusterSpec struct {
	Server   ServerSpec
	Database DatabaseSpec
}

// ServerSpec describes the Temporal Cluster's Server configuration.
type ServerSpec struct {
	Image               string // e.g. "temporalio/server:1.29.2"
	AdminToolsImage     string // e.g. "temporalio/admin-tools:1.29"
	ConfigTemplate      string // YAML template, [[ ]] delimiters for substitution
	SetupSchemaTemplate string // Shell script template, [[ ]] delimiters for substitution
}

// DatabaseSpec describes the Temporal Cluster's Database configuration.
type DatabaseSpec struct {
	Image string // Image is the Docker image for the database container, e.g. "postgres:15".
}
