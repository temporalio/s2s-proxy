#!/bin/bash

set -ex

if [ -z "$CONFIG_YML" ]; then
    echo "Error: CONFIG_YML is not provided."
    exit 1
fi

cat $CONFIG_YML
exec s2s-proxy start --config $CONFIG_YML