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
run_gh() {
  if [[ -z $ghCmd ]]; then
    ghCmd="$(find_tool gh)"
    if [[ -z $ghCmd ]]; then
      error "gh not found in mise environment (run_gh)"
      return 1
    fi
  fi

  "$ghCmd" "$@"
}

# github_token is a convenience wrapper around `gh auth token`.
github_token() {
  run_gh auth token
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

  if [[ $use_pre_releases == "true" ]]; then
    tag=$(run_gh release -R "$slug" list --exclude-drafts | grep Pre-release | head -n1 | awk '{ print $1 }')
  else
    tag=$(run_gh release -R "$slug" list --exclude-drafts | grep -v Pre-release | head -n1 | awk '{ print $1 }')
  fi

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

# Fetch the token from getoutreach/ci:ghaccesstoken if not set.
bootstrap_github_token() {
  local version
  if [[ -z $GITHUB_TOKEN ]]; then
    version="$(get_tool_version getoutreach/ci)"
    if ghaccesstoken_exists "$version"; then
      mise_tool_config_set ubi:getoutreach/ci version "$version" exe ghaccesstoken
      install_tool_with_mise ubi:getoutreach/ci "$version"
    fi
    GITHUB_TOKEN="$("$(find_tool ghaccesstoken)" --skip-update token "$@")"
    export GITHUB_TOKEN
  fi
}

ghaccesstoken_exists() {
  local version="$1"
  local ghaccesstoken_path
  ghaccesstoken_path="$(find_tool ghaccesstoken)"
  test -n "$ghaccesstoken_path" -a "$("$ghaccesstoken_path" --skip-update --version)" -eq "$version"
  return $?
}
