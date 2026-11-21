#!/usr/bin/env bash
# Contains helper functions for the stencil-upgrade job, which opens a
# pull request that upgrades the Stencil modules of a repository.
#
# Requires the following libraries:
# * github.sh (for run_gh)
# * logging.sh (required by github.sh)
# * yq.sh (for yq_wrapper)

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=logging.sh
source "$LIB_DIR/logging.sh"
# shellcheck source=github.sh
source "$LIB_DIR/github.sh"
# shellcheck source=yq.sh
source "$LIB_DIR/yq.sh"

# stencil_upgrade_branch_name echoes the name of the branch that holds
# today's upgrade, which uses the UTC date.
stencil_upgrade_branch_name() {
  echo "stencil-upgrade/$(date -u +%Y-%m-%d)"
}

# stencil_upgrade_open_prs echoes the head branch of each open pull
# request whose branch name starts with the prefix.
#
# $1: The branch prefix, e.g. "stencil-upgrade/".
stencil_upgrade_open_prs() {
  local prefix="$1"
  local branches branch

  # The search matches loosely, so the exact prefix check follows.
  branches="$(run_gh pr list --state open --limit 200 \
    --search "head:${prefix%/}" --json headRefName \
    --jq '.[].headRefName')" || return 1

  while IFS= read -r branch; do
    if [[ $branch == "$prefix"* ]]; then
      echo "$branch"
    fi
  done <<<"$branches"
}

# stencil_upgrade_only_bot_commits returns success when the bot authored
# every commit in base_ref..head_ref. Otherwise, it prints the first
# other commit and returns failure.
#
# $1: The base ref, which the range excludes.
# $2: The head ref.
# $3: The author name of the bot.
# $4: The author email of the bot.
stencil_upgrade_only_bot_commits() {
  local base_ref="$1" head_ref="$2" name="$3" email="$4"
  local commits sha author_name author_email subject

  # Read the log before the loop so that a bad ref fails the function.
  commits="$(git log --format=$'%h\t%an\t%ae\t%s' "$base_ref..$head_ref")" || return 1

  while IFS=$'\t' read -r sha author_name author_email subject; do
    if [[ $author_name != "$name" || $author_email != "$email" ]]; then
      echo "$sha $author_name <$author_email> $subject"
      return 1
    fi
  done <<<"$commits"
}

# stencil_upgrade_force_pr echoes "true" when the job must open a pull
# request even without version changes: when the flag is "true", or when
# the lockfile has a module on the "unstable" channel.
#
# $1: The force flag, "true" or "false".
# $2: The path to the new stencil.lock.
stencil_upgrade_force_pr() {
  local flag="$1" lockfile="$2"

  if [[ $flag == "true" ]]; then
    echo "true"
    return
  fi

  yq_wrapper -r '[.modules[]? | .version == "unstable"] | any' "$lockfile"
}

# stencil_upgrade_version_change_count echoes the number of entries in
# the changes file whose old and new versions differ.
#
# $1: The path to the JSON output of `devbase stencil lock-diff`.
stencil_upgrade_version_change_count() {
  yq_wrapper -r '[.[] | select(.old != .new)] | length' "$1"
}

# stencil_upgrade_decide echoes what the job does next: "open" to open a
# pull request, "stop-no-changes" to stop because nothing changed, or
# "stop-empty" to stop because a forced pull request would be empty.
#
# $1: The number of version changes.
# $2: Whether the pull request is forced, "true" or "false".
# $3: Whether the working tree has changes, "true" or "false".
stencil_upgrade_decide() {
  local version_change_count="$1" force_pr="$2" tree_dirty="$3"

  if ((version_change_count > 0)); then
    echo "open"
  elif [[ $force_pr != "true" ]]; then
    echo "stop-no-changes"
  elif [[ $tree_dirty == "true" ]]; then
    echo "open"
  else
    echo "stop-empty"
  fi
}
