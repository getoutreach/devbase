#!/usr/bin/env bash
# Installs 'docker buildx' if it doesn't already exist. On Linux,
# creates a buildx instance and boots it.

# BUILDX_VERSION is the version of 'docker buildx' we should use if not
# already installed on the host system.
BUILDX_VERSION="v0.11.2"

# ARCH is the architecture of the host system. Matches the format,
# loosely, of GOARCH.
ARCH=$(uname -m)
if [[ $ARCH == "x86_64" ]]; then
  ARCH="amd64"
elif [[ $ARCH == "aarch64" ]]; then
  ARCH="arm64"
fi

# BUILDX_BINARY_URL is the URL to download the buildx binary from.
BUILDX_BINARY_URL="https://github.com/docker/buildx/releases/download/$BUILDX_VERSION/buildx-$BUILDX_VERSION.linux-$ARCH"

# only install buildx when we don't already have it
if ! docker buildx version >/dev/null 2>&1; then
  echo "👉 Installing Buildx"
  curl --output docker-buildx \
    --silent --show-error --location --fail --retry 3 \
    "$BUILDX_BINARY_URL"

  mkdir -p ~/.docker/cli-plugins
  mv docker-buildx ~/.docker/cli-plugins/
  chmod a+x ~/.docker/cli-plugins/docker-buildx
fi

# On macOS we don't need to create a builder or support QEMU.
if [[ $OSTYPE == "linux-gnu"* ]]; then
  # Take from setup-buildx Github Action
  echo "� Creating a new builder instance"
  docker buildx create --use --name devbase

  echo "🏃 Booting builder"
  docker buildx inspect --bootstrap
fi
