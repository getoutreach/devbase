#!/usr/bin/env bash
# Helpers for installing devenv in CI.

# install_devenv_for_ci installs the devenv version CI should use:
#
# 1. DEVENV_CI_VERSION, if set and non-empty (with or without a leading `v`)
# 2. the newest release of any kind, if DEVENV_PRE_RELEASE=true
# 3. otherwise, the latest stable release
#
# It always installs, so a devenv restored from a tool cache never wins.
# Requires github.sh, logging.sh, mise.sh and version.sh to be sourced.
install_devenv_for_ci() {
  local version
  version="$(normalize_semver "${DEVENV_CI_VERSION:-}")"
  local pre_release="${DEVENV_PRE_RELEASE:-false}"

  if [[ -n $version ]]; then
    if ! is_semver "$version"; then
      error "DEVENV_CI_VERSION must be a devenv release version such as 1.68.0, got '${DEVENV_CI_VERSION}'"
      return 1
    fi
    if [[ $pre_release == "true" ]]; then
      warn "Ignoring DEVENV_PRE_RELEASE because DEVENV_CI_VERSION is set"
    fi
    info "Using devenv $version (from DEVENV_CI_VERSION)"
    # mise reads GITHUB_TOKEN when it resolves the release assets.
    bootstrap_github_token
    install_tool_with_mise github:getoutreach/devenv "$version"
  elif [[ $pre_release == "true" ]]; then
    info "Using the latest devenv pre-release (DEVENV_PRE_RELEASE=true)"
    install_latest_github_release getoutreach/devenv true
  else
    info "Using the latest stable devenv release (DEVENV_CI_VERSION not set)"
    install_latest_github_release getoutreach/devenv false
  fi
}
