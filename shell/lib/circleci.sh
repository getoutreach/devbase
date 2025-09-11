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
