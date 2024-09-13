#!/bin/bash

set -ex

cat $CONFIG_YML
exec s2s-proxy start --config $CONFIG_YML