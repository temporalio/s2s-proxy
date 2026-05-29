#!/bin/bash

# Generates the cert/key pairs referenced by:
#   develop/config/local-test-config-server.yaml
#   develop/config/local-test-config-client.yaml
#
# Both configs load TLS material from ./proxycerts/ relative to the proxy's
# working directory. Each side uses the peer's .pem as its remoteCAPath, so the
# certs are self-signed (CA:TRUE), matching the pattern in generate-certs.sh.

mkdir -p ./proxycerts

echo "Generating Cluster Certificate for onebox-proxy-server.cluster.tmprl.cloud"
openssl req -x509 -newkey rsa:4096 -keyout proxycerts/server.key -out proxycerts/server.pem -days 365 -nodes -addext "subjectAltName = DNS:onebox-proxy-server.cluster.tmprl.cloud" -subj "/C=US/ST=WA/O=Temporal/CN=onebox-proxy-server.cluster.tmprl.cloud"

echo "Generating Cluster Certificate for onebox-proxy-client.cluster.tmprl.cloud"
openssl req -x509 -newkey rsa:4096 -keyout proxycerts/client.key -out proxycerts/client.pem -days 365 -nodes -addext "subjectAltName = DNS:onebox-proxy-client.cluster.tmprl.cloud" -subj "/C=US/ST=WA/O=Temporal/CN=onebox-proxy-client.cluster.tmprl.cloud"
