#!/usr/bin/env bash
# Sets up a machine in CircleCI

# remove the existing yq, if it already exists
sudo rm "$(command -v yq)" || true

# yq
echo "Installing yq"
pip3 install yq

# Vault
if ! command -v vault >/dev/null; then
  echo "Installing Vault"
  curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
  sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
  sudo apt-get update
  sudo apt-get install -y vault
fi
