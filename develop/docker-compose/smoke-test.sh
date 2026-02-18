#!/bin/sh
set -e

sleep 20 # TODO: remove after addition of reliable s2s-proxy healthcheck endpoint.

# connectivity (also covered by docker-compose healthcheck)
temporal operator cluster health --address temporal-left:7233
temporal operator cluster health --address temporal-right:7233

# cross-server proxy-left routes to temporal-right
NS_L="proxy-left-$$"
temporal operator namespace create "$NS_L" --retention 24h --address temporal-right:7233
temporal operator namespace describe "$NS_L" --address proxy-left:6233

# cross-server proxy-right routes to temporal-left
NS_R="proxy-right-$$"
temporal operator namespace create "$NS_R" --retention 24h --address temporal-left:7233
temporal operator namespace describe "$NS_R" --address proxy-right:6333

echo "smoke tests succeeded. 🚬"
