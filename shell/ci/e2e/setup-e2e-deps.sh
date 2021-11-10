#!/usr/bin/env bash
# Setup dependencies for e2e tests

# Vault
curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add -
apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
apt-get update
apt-get install -y vault

# kubecfg
curl -fsSL https://github.com/getoutreach/kubecfg/releases/download/v0.17.0/kubecfg-linux-amd64 >"/usr/local/bin/kubecfg"
chmod +x /usr/local/bin/kubecfg
