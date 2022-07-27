#!/usr/bin/env bash
BUILDX_VERSION="v0.8.2"
BUILDX_BINARY_URL="https://github.com/docker/buildx/releases/download/$BUILDX_VERSION/buildx-$BUILDX_VERSION.linux-amd64"

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

# On macOS we don't need to create a builder or support QEMU
if [[ $OSTYPE == "linux-gnu"* ]]; then
  # Taken from setup-qemu Github Action
  echo "💎 Installing QEMU static binaries..."
  docker run --rm --privileged tonistiigi/binfmt:latest --install all

  echo "� Extracting available platforms..."
  docker run --rm --privileged tonistiigi/binfmt:latest

  # Take from setup-buildx Github Action
  echo "� Creating a new builder instance"
  docker buildx create --use

  echo "🏃 Booting builder"
  docker buildx inspect --bootstrap
fi
