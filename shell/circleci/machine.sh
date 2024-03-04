#!/usr/bin/env bash
# Install dependencies that are required in a machine environment.
# These are usually already installed in a CircleCI docker image.
set -e

# ARCH is the current architecture of the machine. Valid values are:
#   - amd64
#   - arm64
ARCH="amd64"
if [[ "$(uname -m)" == "aarch64" ]]; then
  ARCH="arm64"
fi

# GH_VERSION is the version of gh to install.
export GH_VERSION=2.32.1

# should_install_vault is a helper function that checks if the vault
# binary is already installed, if so it returns false. Otherwise, it
# returns true.
should_install_vault() {
  ! command -v vault >/dev/null 2>&1
}

# Remove APT repositories we don't need.
# We remove the heroku apt repository because we don't
# heroku, but also due to a potential hostile takeover
# on Nov 14th, 2022.
if [[ -e "/etc/apt/sources.list.d/heroku.list" ]]; then
  sudo rm /etc/apt/sources.list.d/heroku.list
fi

# Add APT repositories we do need.
if should_install_vault; then
  wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o hashicorp-archive-keyring.gpg
  sudo mv hashicorp-archive-keyring.gpg /usr/share/keyrings/
  echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
fi

# Rebuild the apt list
sudo apt-get update -y

if ! command -v gh >/dev/null; then
  echo "Installing gh"

  wget -O gh.deb https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${ARCH}.deb
  sudo apt install -yf ./gh.deb
  rm ./gh.deb
fi

echo "Installing yq"
# Remove the existing yq, if it already exists
# (usually the Go Version we don't support)
sudo rm -f "$(command -v yq)"

# Use apt to install yq, if available. Otherwise, fallback to pip3 which
# older versions of Ubuntu/Debian support.
if ! apt install -y yq; then
  # Install Python and pip
  if ! command -v pip3 >/dev/null; then
    echo "Installing pip3"
    sudo apt-get install --no-install-recommends -y python3-pip
  fi

  echo "Failed to install yq through apt, falling back to pip3 install (this only works on Ubuntu 22.04 and below)"
  sudo pip3 install yq
fi

if should_install_vault; then
  echo "Installing Vault"
  sudo apt-get install -y vault
  sudo rm -rf /opt/vault
fi
