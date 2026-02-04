# Claude Code Guidelines for s2s-proxy

## Code Style & Conventions

### License Headers
- **DO NOT** add MIT License headers to new files
- Keep new files without license headers at the top
- Existing files with license headers should be left as-is

## Project Overview

This is the s2s-proxy project for Temporal, providing server-to-server proxy functionality.

### Important Components

#### Config Converter Tool
- Located at: `cmd/tools/configconverter/`
- Purpose: Converts legacy s2s-proxy configs to new `clusterConnections` format
- Uses `config.ToClusterConnConfig()` for conversion
- Filters out deprecated fields marked with "TODO: Soon to be deprecated!"
