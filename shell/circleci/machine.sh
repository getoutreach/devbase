#!/usr/bin/env bash
# Install dependencies that are required in a machine environment.
# These are usually already installed in a CircleCI docker image.
set -e

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

  # 2023-02-22: Hashicorp updated their keyring but removed their older key, this reulted in no
  # being able to fetch their repository. To make up for this, we fetch the older key on the
  # fly and add it to their keyring. This can be removed once they have resolved the following
  # issue: https://github.com/hashicorp/vault/issues/19292
  #
  # This is also best effort, so we ignore any errors.
  gpg --no-default-keyring --keyring ./hashicorp-archive-keyring.gpg --keyserver keyserver.ubuntu.com --recv-keys DA418C88A3219F7B || true
  echo " === Hashicorp Keyring ==="
  gpg --no-default-keyring --keyring ./hashicorp-archive-keyring.gpg --list-keys || true
  echo " === End Hashicorp Keyring ==="
  sudo mv hashicorp-archive-keyring.gpg /usr/share/keyrings/

  echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
fi

# Rebuild the apt list
sudo apt-get update -y

# Install Python and pip
if ! command -v pip3 >/dev/null; then
  echo "Installing pip3"
  sudo apt-get install --no-install-recommends -y python3-pip
fi

if ! command -v gh >/dev/null; then
  echo "Installing gh"
  wget -O gh.deb https://github.com/cli/cli/releases/download/v2.20.0/gh_2.20.0_linux_amd64.deb
  sudo apt install -yf ./gh.deb
  rm gh.deb
fi

echo "Installing yq"
# Remove the existing yq, if it already exists
# (usually the Go Version we don't support)
sudo rm "$(command -v yq)" || true
sudo pip3 install yq

# Vault
if should_install_vault; then
  echo "Installing Vault"
  sudo apt-get install -y vault
fi
