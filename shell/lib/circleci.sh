#!/usr/bin/env bash
#
# CircleCI-specific functions

# Determines whether we're currently running in a PR job in CircleCI.
circleci_is_pr() {
  [[ -n $CIRCLE_PULL_REQUEST ]]
}

# Determines whether we're currently running in a PR job in CircleCI,
# where the source of the PR is from a fork.
circleci_pr_is_fork() {
  [[ -n $CIRCLE_PR_REPONAME ]]
}

# Returns 0 if VALUE is a truthy CircleCI bool-parameter substitution
# (`1`, `true`, `TRUE`, `True`), 1 otherwise. CircleCI substitutes a YAML
# `true` bool parameter into `environment:` mappings as the string `"1"`.
# See https://circleci.com/docs/reusing-config/#parameter-syntax.
circleci_param_is_true() {
  case "${1:-}" in
  1 | true | TRUE | True) return 0 ;;
  *) return 1 ;;
  esac
}

# Returns 0 if the job is running in E2E mode (i.e. the `e2e_mode` bool
# parameter was set to `true` and exported as `E2E_MODE`).
circleci_e2e_mode_enabled() {
  circleci_param_is_true "${E2E_MODE:-}"
}
