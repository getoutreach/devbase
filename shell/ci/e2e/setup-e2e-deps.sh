#!/usr/bin/env bash
# Setup dependencies for e2e tests
set -e

# kubecfg
curl -fsSL https://github.com/getoutreach/kubecfg/releases/download/v0.17.0/kubecfg-linux-amd64 >"/usr/local/bin/kubecfg"
chmod +x /usr/local/bin/kubecfg
