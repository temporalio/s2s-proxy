set -e
export SQL_PASSWORD=temporal
T="temporal-sql-tool --plugin postgres12 --ep [[.DatabaseAlias]] -u temporal -p 5432"

$T --db temporal       setup-schema -v 0.0
$T --db temporal       update-schema -d /etc/temporal/schema/postgresql/v12/temporal/versioned

$T --db temporal_visibility create
$T --db temporal_visibility setup-schema -v 0.0
$T --db temporal_visibility update-schema -d /etc/temporal/schema/postgresql/v12/visibility/versioned

echo "temporal schema setup complete"
