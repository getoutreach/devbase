#!/usr/bin/env bash
# Contains various helper functions for interacting with Github.

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=logging.sh
source "$LIB_DIR/logging.sh"
# shellcheck source=mise.sh
source "$LIB_DIR/mise.sh"
# shellcheck source=shell.sh
source "$LIB_DIR/shell.sh"

ghCmd=""

# Looks for gh in the PATH or in the mise environment.
gh_installed() {
  if [[ -z $ghCmd ]]; then
    ghCmd="$(find_tool gh)"
    if [[ -z $ghCmd ]]; then
      error "gh not found in mise environment (gh_installed)"
      return 1
    fi
  fi
}

# Runs gh if found, otherwise fails.
run_gh() {
  if ! gh_installed; then
    return 1
  fi

  "$ghCmd" "$@"
}

# github_token is a convenience wrapper around `gh auth token`.
github_token() {
  run_gh auth token
}

# Determines the latest release version of a GitHub repository.
latest_github_release_version() {
  local slug="$1"
  local use_pre_releases="$2"

  local gh_args=(--limit 1 --json tagName --jq '.[].tagName' --exclude-drafts)
  if [[ $use_pre_releases != "true" ]]; then
    gh_args+=(--exclude-pre-releases)
  fi

  run_gh release --repo "$slug" list "${gh_args[@]}"
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
install_latest_github_release() {
  local slug="$1"
  local use_pre_releases="$2"
  local binary_name=${3:-$(basename "$slug")}
  local tag

  tag="$(latest_github_release_version "$slug" "$use_pre_releases")"

  # If we have an empty tag, something went wrong. Fail.
  if [[ -z $tag ]]; then
    error "Failed to determine version for $slug (install_latest_github_release)"
    return 1
  fi

  info "Using $slug:${binary_name} version: ($tag)"

  # Need to export GITHUB_TOKEN so that future calls to `mise`
  # continue to use it for the configured private repos.
  if [[ -z ${GITHUB_TOKEN:-} ]]; then
    GITHUB_TOKEN="$(github_token)"
    export GITHUB_TOKEN
  fi

  local mise_identifier="ubi:$slug"
  # If binary_name is not the default value, set the exe parameter in
  # the mise config.
  if [[ -n $3 ]]; then
    mise_tool_config_set "$mise_identifier" version "$tag" exe "$binary_name"
  fi
  install_tool_with_mise "$mise_identifier" "$tag"
}

# Set GITHUB_TOKEN from getoutreach/ci:ghaccesstoken if not already
# set. Any arguments are passed to `ghaccesstoken token`.
bootstrap_github_token() {
  if [[ -z ${GITHUB_TOKEN:-} ]]; then
    GITHUB_TOKEN="$(fetch_github_token_from_ci "$@")"
    export GITHUB_TOKEN
  fi
}

# Print the GitHub token from getoutreach/ci:ghaccesstoken. Any
# arguments are passed to `ghaccesstoken token`.
# Requires lib/bootstrap.sh for `get_tool_version`.
fetch_github_token_from_ci() {
  (
    local version
    version="$(get_tool_version getoutreach/ci)"
    if ! ghaccesstoken_exists "$version"; then
      mise_tool_config_set ubi:getoutreach/ci version "$version" exe ghaccesstoken
      install_tool_with_mise ubi:getoutreach/ci "$version"
    fi
  ) >&2
  "$(find_tool ghaccesstoken)" --skip-update token "$@"
}

# Determines whether ghaccesstoken is installed with the provided
# version number.
ghaccesstoken_exists() {
  local version="$1"
  local ghaccesstoken_path
  ghaccesstoken_path="$(mise which ghaccesstoken 2>/dev/null)"
  if [[ -z $ghaccesstoken_path ]]; then
    return 1
  fi
  [[ "$("$ghaccesstoken_path" --skip-update --version | awk '{print $3}')" == "$version" ]]
}
