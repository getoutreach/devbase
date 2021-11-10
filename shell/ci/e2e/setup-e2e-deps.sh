#!/usr/bin/env bash
# Setup dependencies for e2e tests

# Vault
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update
sudo apt-get install -y vault

# kubecfg
curl -fsSL https://github.com/getoutreach/kubecfg/releases/download/v0.17.0/kubecfg-linux-amd64 >"/usr/local/bin/kubecfg"
