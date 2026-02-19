#!/bin/sh
set -e

sleep 20 # TODO: remove after addition of reliable s2s-proxy healthcheck endpoint.

# connectivity (also covered by docker-compose healthcheck)
temporal operator cluster health --address "$TEMPORAL_LEFT_INBOUND"
temporal operator cluster health --address "$TEMPORAL_RIGHT_INBOUND"

# cross-server proxy-left routes to temporal-right
NS_L="smoketest-left-$$"
temporal operator namespace create "$NS_L" --retention 24h --address "$TEMPORAL_RIGHT_INBOUND"
temporal operator namespace describe "$NS_L" --address "$PROXY_LEFT_OUTBOUND"

# cross-server proxy-right routes to temporal-left
NS_R="smoketest-right-$$"
temporal operator namespace create "$NS_R" --retention 24h --address "$TEMPORAL_LEFT_INBOUND"
temporal operator namespace describe "$NS_R" --address "$PROXY_RIGHT_OUTBOUND"

# outbound route is routed through each proxy
# TODO: for strong validation - after migrating off of temporal cli dev server, we can override and check the cluster name (currently both are named 'active')
temporal operator cluster describe --address "$PROXY_LEFT_OUTBOUND"
temporal operator cluster describe --address "$PROXY_RIGHT_OUTBOUND"

echo "smoke tests succeeded. 🚬"
