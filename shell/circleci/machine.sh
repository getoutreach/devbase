#!/usr/bin/env bash
# Install dependencies that are required in a machine environment.
# These are usually already installed in a CircleCI docker image.

# Install Python and pip
if ! command -v pip3 >/dev/null; then
  echo "Installing pip3"
  sudo apt-get update -y
  sudo apt-get install --no-install-recommends -y python3-pip
  sudo apt-get clean -y
fi

if ! command -v gh >/dev/null; then
  echo "Installing gh"
  wget -O gh.deb https://github.com/cli/cli/releases/download/v2.5.0/gh_2.5.0_linux_amd64.deb
  sudo apt install -yf ./gh.deb
  rm gh.deb
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
