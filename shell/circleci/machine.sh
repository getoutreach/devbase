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
  wget -O gh.deb https://github.com/cli/cli/releases/download/v2.12.1/gh_2.12.1_linux_amd64.deb
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
  wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor | sudo tee /usr/share/keyrings/hashicorp-archive-keyring.gpg >/dev/null
  echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
  sudo apt-get update
  sudo apt-get install -y vault
fi
