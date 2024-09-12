#!/usr/bin/env bash

set -euo pipefail

SRC=$1
# Default to user home dir on the remote
TGT=$2

DNS=$(terraform output -raw public_dns)
set -x
exec scp -r -i ~/.ssh/pglass-s2c-id-rsa "$SRC" "ec2-user@${DNS}:$TGT"

