#!/usr/bin/env bash
# Sets up a machine in CircleCI

# Install Python and pip
if ! command -v pip3 >/dev/null; then
  sudo apt-get update -y
  sudo apt-get install --no-install-recommends -y python3-pip
  sudo apt-get clean -y
fi

echo "Installing yq"
# remove the existing yq, if it already exists
sudo rm "$(command -v yq)" || true
sudo pip3 install yq

# Vault
if ! command -v vault >/dev/null; then
  echo "Installing Vault"
  curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
  sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
  sudo apt-get update
  sudo apt-get install -y vault
fi
