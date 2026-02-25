#!/bin/sh
set -e

sleep 20 # TODO: remove after addition of reliable s2s-proxy healthcheck endpoint.

# connectivity (also covered by docker-compose healthcheck)
temporal operator cluster health --address "$TEMPORAL_A_INBOUND"
temporal operator cluster health --address "$TEMPORAL_B_INBOUND"

# cross-server proxy-a routes to temporal-b
NS_A="smoketest-a-$$"
temporal operator namespace create "$NS_A" --retention 24h --address "$TEMPORAL_B_INBOUND"
temporal operator namespace describe "$NS_A" --address "$PROXY_A_OUTBOUND"

# cross-server proxy-b routes to temporal-a
NS_B="smoketest-b-$$"
temporal operator namespace create "$NS_B" --retention 24h --address "$TEMPORAL_A_INBOUND"
temporal operator namespace describe "$NS_B" --address "$PROXY_B_OUTBOUND"

# outbound route is routed through each proxy
# TODO: for strong validation - after migrating off of temporal cli dev server, we can override and check the cluster name (currently both are named 'active')
temporal operator cluster describe --address "$PROXY_A_OUTBOUND"
temporal operator cluster describe --address "$PROXY_B_OUTBOUND"

echo "smoke tests succeeded. 🚬"
