#!/usr/bin/env bash
# Contains various helper functions for interacting with Github.

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=asdf.sh
source "$LIB_DIR/asdf.sh"
# shellcheck source=logging.sh
source "$LIB_DIR/logging.sh"

ghCmd=""

run_gh() {
  if [[ -z $ghCmd ]]; then
    ghCmd="$(command -v gh)"
    if [[ -z $ghCmd ]]; then
      if ! command -v mise >/dev/null; then
        echo "Error: gh and mise not found (run_gh)" >&2
        return 1
      fi
      ghCmd="$(mise which gh)"
    fi
  fi

  "$ghCmd" "$@"
}

# install_latest_github_release downloads the latest version of a tool
# from Github. Requires the 'gh' cli to be installed either directly or via mise.
#
# $1: The slug of the repo to download from. This is the same as the
#     repo name, e.g. "github/hub".
# $2: Whether or not to use pre-releases. If "true", will download the
#     latest pre-release. If "false", or empty, will download the
#     latest stable release. "Pre-release", for Outreach releasing, is
#     any release that is not marked stable (e.g., unstable or rc).
# $3: The name of the binary to extract from the downloaded archive. If
#     empty, will use the basename of the slug.
# $4: The directory to install the binary to. If empty, will use
#     /usr/local/bin.
install_latest_github_release() {
  local slug="$1"
  local use_pre_releases="$2"
  local binary_name=${3:-$(basename "$slug")}
  local install_dir=${4:-/usr/local/bin}

  if [[ $use_pre_releases == "true" ]]; then
    # shellcheck disable=SC2155 # Why: We're OK with this being
    # potentially masked.
    local tag=$(run_gh release -R "$slug" list --exclude-drafts | grep Pre-release | head -n1 | awk '{ print $1 }')
  else
    # shellcheck disable=SC2155 # Why: We're OK with this being
    # potentially masked.
    local tag=$(run_gh release -R "$slug" list --exclude-drafts | grep -v Pre-release | head -n1 | awk '{ print $1 }')
  fi

  # If we have an empty tag, something went wrong. Fail.
  if [[ -z $tag ]]; then
    echo "Error: failed to determine version for $slug (install_github_release)" >&2
    return 1
  fi

  # shellcheck disable=SC2155 # Why: We're OK with this being
  # potentially masked.
  local GOOS=$(asdf_devbase_run go env GOOS)

  # shellcheck disable=SC2155 # Why: We're OK with this being
  # potentially masked.
  local GOARCH=$(asdf_devbase_run go env GOARCH)

  info "Using $slug:${binary_name} version: ($tag)"

  tmpDir=$(mktemp -d)
  pushd "$tmpDir" >/dev/null || return 1

  # Download the release and extract the binary.
  #
  # Note: we use basename here intentionally because all of the
  # configuration for the releasers will release based on the repo
  # basename, not the binary name.
  #
  # shellcheck disable=SC2155 # Why: We're OK with this being
  # potentially masked.
  local repoName=$(basename "$slug")
  local pattern="${repoName}_*_${GOOS}_$GOARCH.tar.gz"

  # Download the release through the Github CLI.
  run_gh release -R "$slug" download "$tag" --pattern "$pattern"

  echo "" # Fixes issues with output being corrupted in CI
  tar xf "${repoName}"**.tar.*

  # If not writable, use sudo.
  baseArgs=()
  if [[ ! -w $install_dir ]]; then
    baseArgs=(sudo)
  fi

  # Move the binary to the install directory and ensure it's owned by
  # the current user.
  "${baseArgs[@]}" mv "$binary_name" "$install_dir/$binary_name"
  "${baseArgs[@]}" chown "$(id -u):$(id -g)" "$install_dir/$binary_name"

  popd >/dev/null || return 1
  rm -rf "$tmpDir"
}
