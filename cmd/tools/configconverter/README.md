# Config Converter Tool

A command-line tool to convert s2s-proxy configuration files from the legacy format to the new `clusterConnections` format.

## Overview

This tool reads a config file in YAML format (legacy format with separate `inbound`, `outbound`, and `mux` sections) and converts it to the new format using the `ToClusterConnConfig` function from `config/converter.go`. The converted config is then exported to a new YAML file.

## Building

```bash
go build -o configconverter ./cmd/tools/configconverter
```

## Usage

```bash
configconverter -input <input-file> -output <output-file>
```

### Flags

- `-input`: Path to the input config file in YAML format (required)
- `-output`: Path to the output config file in YAML format (required)

### Example

```bash
# Convert a legacy config to new format
configconverter -input ./develop/legacyconfig/old-config-with-override.yaml -output ./new-config.yaml
```

## What It Does

The tool performs the following steps:

1. Reads the legacy config file using `config.LoadConfig`
2. Converts it to the new format using `config.ToClusterConnConfig`
3. Filters out deprecated fields (marked with "TODO: Soon to be deprecated!")
4. Writes only the non-deprecated fields to the output file

### Excluded Fields

The following deprecated fields are automatically excluded from the output:
- `inbound`
- `outbound`
- `mux`
- `healthCheck`
- `outboundHealthCheck`
- `shardCount` (top-level)
- `memberlist` (top-level)

These settings are migrated into the `clusterConnections` array in the new format.

## Legacy vs New Format

### Legacy Format

The legacy format uses separate sections:
- `inbound`: Configuration for the inbound proxy
- `outbound`: Configuration for the outbound proxy
- `mux`: Multiplexing transport configuration

### New Format

The new format uses a `clusterConnections` array where each entry contains:
- `name`: Connection name (combined from inbound/outbound names)
- `local`: Local cluster configuration
- `remote`: Remote cluster configuration
- Additional settings like ACL policies, namespace translation, etc.

## Testing

You can test the converter with the example legacy configs in `./develop/legacyconfig/`:
- `old-config-with-override.yaml`
- `old-config-with-TLS.yaml`
- `empty-config.yaml`

```bash
configconverter -input ./develop/legacyconfig/old-config-with-override.yaml -output /tmp/converted.yaml
```
