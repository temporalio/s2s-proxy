#!/bin/bash

mkdir -p ./develop/certificates
echo "Generating Cluster Certificate for onebox-proxy1.cluster.tmprl.cloud"
openssl req -x509 -newkey rsa:4096 -keyout develop/certificates/proxy1.key -out develop/certificates/proxy1.pem -days 365 -nodes -addext "subjectAltName = DNS:onebox-proxy1.cluster.tmprl.cloud" -subj "/C=US/ST=WA/O=Temporal/CN=onebox-proxy1.cluster.tmprl.cloud"

echo "Generating Cluster Certificate for onebox-proxy2.cluster.tmprl.cloud"
openssl req -x509 -newkey rsa:4096 -keyout develop/certificates/proxy2.key -out develop/certificates/proxy2.pem -days 365 -nodes -addext "subjectAltName = DNS:onebox-proxy2.cluster.tmprl.cloud" -subj "/C=US/ST=WA/O=Temporal/CN=onebox-proxy2.cluster.tmprl.cloud"

echo "Added yml files to develop/secrets"