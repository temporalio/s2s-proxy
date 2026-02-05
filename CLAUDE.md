# Claude Code Guidelines for s2s-proxy

## Code Style & Conventions

### License Headers
- **DO NOT** add MIT License headers to new files
- Keep new files without license headers at the top
- Existing files with license headers should be left as-is

### Linting
- **ALWAYS** ensure code passes linting before committing
- Run: `make lint` or `go tool -modfile=develop/tools.mod golangci-lint run --build-tags=test_dep`
- Fix any linting errors before pushing commits

### Import Formatting
- Follow `gci` (goimports-reviser) formatting rules:
  1. Standard library imports
  2. Third-party imports
  3. Project imports (github.com/temporalio/s2s-proxy/...)
- Add blank lines between import groups

## Project Overview

This is the s2s-proxy project for Temporal, providing server-to-server proxy functionality.

### Important Components

#### Config Converter Tool
- Located at: `cmd/tools/configconverter/`
- Purpose: Converts legacy s2s-proxy configs to new `clusterConnections` format
- Uses `config.ToClusterConnConfig()` for conversion
- Filters out deprecated fields marked with "TODO: Soon to be deprecated!"
