#!/usr/bin/env bash
# Contains various helper functions for interacting with Github.
#
# Requires the following libraries:
# * logging.sh (required by mise.sh)
# * mise.sh
# * shell.sh (required by mise.sh)
#
# Setting the GitHub token requires bootstrap.sh.

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
#
# This uses the REST API rather than `gh release list` because the
# latter relies on GitHub's GraphQL API, which intermittently returns
# 401 Unauthorized for some organization repositories.
latest_github_release_version() {
  local slug="$1"
  local use_pre_releases="$2"

  # Filter out drafts, and pre-releases unless they're requested, then
  # print the tag name of the most recent matching release. Releases are
  # returned newest-first by the API.
  local jq_filter='[.[] | select(.draft == false)'
  if [[ $use_pre_releases != "true" ]]; then
    jq_filter+=' | select(.prerelease == false)'
  fi
  jq_filter+='] | first | .tag_name // empty'

  run_gh api "repos/$slug/releases" --jq "$jq_filter"
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
    if [[ -z $GITHUB_TOKEN ]]; then
      # shellcheck disable=SC2119
      # Why: no extra args needed to pass to ghaccesstoken in this case.
      bootstrap_github_token
    fi
    export GITHUB_TOKEN
  fi

  local mise_identifier="github:$slug"
  install_tool_with_mise "$mise_identifier" "$tag"

  if [[ -n ${3:-} ]] && ! find_tool "$3"; then
    error "Expecting to install '$3' but was not installed"
    return 1
  fi
}

# Set GITHUB_TOKEN from getoutreach/ci:ghaccesstoken if not already
# set. Any arguments are passed to `ghaccesstoken token`.
# shellcheck disable=SC2120
# Why: External scripts using this library file could pass the arguments.
bootstrap_github_token() {
  if [[ -z ${GITHUB_TOKEN:-} ]]; then
    GITHUB_TOKEN="$(fetch_github_token_from_ci "$@")"
    export GITHUB_TOKEN
  fi
}

# Print the GitHub token from getoutreach/ci:ghaccesstoken. Any
# arguments are passed to `ghaccesstoken token`.
fetch_github_token_from_ci() {
  mise_exec_tool_with_bin github:getoutreach/ci ghaccesstoken --skip-update token "$@"
}

GITHUB_PAT=""
# Print the GitHub PAT from getoutreach/ci:ghaccesstoken.
# Cached in `$GITHUB_PAT`.
github_pat_from_ci() {
  if [[ -z $GITHUB_PAT ]]; then
    set +e
    GITHUB_PAT="$(fetch_github_token_from_ci --env-prefix GHACCESSTOKEN_PAT)"
    local exitCode=$?
    set -e
    if [[ $exitCode != 0 ]]; then
      fatal "Could not fetch non-ratelimited GitHub PAT used for GitHub Packages access, try again later"
    elif [[ -z $GITHUB_PAT ]]; then
      fatal "Could not fetch GitHub PAT used for GitHub Packages access, try again later"
    fi
  fi

  echo "$GITHUB_PAT"
}
