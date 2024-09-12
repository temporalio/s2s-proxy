#!/usr/bin/env bash

set -euo pipefail

DNS=$(terraform output -raw public_dns)
set -x
exec ssh -i ~/.ssh/pglass-s2c-id-rsa "ec2-user@${DNS}"
