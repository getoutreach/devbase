#!/usr/bin/env bash
# Contains helper functions for the stencil-upgrade job, which opens a
# pull request that upgrades the Stencil modules of a repository.
#
# Requires the following libraries:
# * box.sh (for get_box_field)
# * cli.sh (for devbase_cli, sourced by yq.sh)
# * github.sh (for run_gh and mise_exec_tool_with_bin)
# * logging.sh (required by github.sh)
# * yq.sh (for yq_wrapper)

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=box.sh
source "$LIB_DIR/box.sh"
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
# request whose branch name starts with the prefix. It ignores pull
# requests from forks.
#
# $1: The branch prefix, e.g. "stencil-upgrade/".
stencil_upgrade_open_prs() {
  local prefix="$1"
  local branches branch

  # The search matches loosely, so the exact prefix check follows.
  branches="$(run_gh pr list --state open --limit 200 \
    --search "head:${prefix%/}" --json headRefName,isCrossRepository \
    --jq '.[] | select(.isCrossRepository | not) | .headRefName')" || return 1

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
  # An empty range has no other author. Without this, read would see one
  # empty line and report a commit that does not exist.
  if [[ -z $commits ]]; then
    return 0
  fi

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

# stencil_upgrade_default_branch echoes the default branch of origin. It
# reads origin/HEAD, then asks origin, and echoes "main" when both fail.
stencil_upgrade_default_branch() {
  local ref name
  if ref="$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null)"; then
    echo "${ref#origin/}"
    return
  fi

  # The output has the line "ref: refs/heads/<name><TAB>HEAD".
  while IFS=$'\t' read -r ref name; do
    if [[ $ref == "ref: refs/heads/"* && $name == "HEAD" ]]; then
      echo "${ref#ref: refs/heads/}"
      return
    fi
  done < <(git ls-remote --symref origin HEAD 2>/dev/null)
  echo "main"
}

# stencil_upgrade_fetch fetches each branch from origin to its
# remote-tracking ref. The refspecs are explicit because a CI checkout
# can fetch only some branches by default.
#
# $@: The branches.
stencil_upgrade_fetch() {
  local branch refspecs=()
  for branch in "$@"; do
    refspecs+=("+refs/heads/$branch:refs/remotes/origin/$branch")
  done
  git fetch -q origin "${refspecs[@]}"
}

# stencil_upgrade_stencil_version echoes the version of stencil, without
# the "stencil version" prefix.
stencil_upgrade_stencil_version() {
  local output
  output="$(mise_exec_tool_with_bin github:getoutreach/stencil stencil --skip-update --version)" || return 1
  echo "${output#stencil version }"
}

# stencil_upgrade_stencil runs the stencil upgrade in the checkout, then
# writes the lock-diff JSON between the stencil.lock of the default
# branch and the new stencil.lock to changes.json in the work directory.
# The diff starts from the default branch so that it lists every module
# change in the pull request.
#
# $1: The work directory, outside of the checkout.
# $2: The default branch, fetched to origin/<name>.
stencil_upgrade_stencil() {
  local work_dir="$1" default_branch="$2"
  local old_lock="$work_dir/old.lock" checkout origin_url

  git show "origin/$default_branch:stencil.lock" >"$old_lock" || return 1
  checkout="$(git rev-parse --show-toplevel)" || return 1
  origin_url="$(git remote get-url origin)" || return 1
  # Stencil module repositories need their own build before stencil runs.
  if [[ "$(basename "$origin_url" .git)" == stencil-* ]]; then
    make || return 1
  fi
  # mise refuses to run tasks from a config that it does not trust.
  MISE_TRUSTED_CONFIG_PATHS="$checkout" mise run stencil:upgrade || return 1

  # devbase_cli can replace its shell, so it runs in a subshell.
  (devbase_cli stencil lock-diff --old "$old_lock" --new stencil.lock) >"$work_dir/changes.json"
}

# stencil_upgrade_is_dry_run succeeds when STENCIL_UPGRADE_DRYRUN is
# "true".
stencil_upgrade_is_dry_run() {
  [[ ${STENCIL_UPGRADE_DRYRUN:-false} == "true" ]]
}

# stencil_upgrade_commit writes the pull request body to body.md in the
# work directory, then commits all changes with the title. In a dry run,
# it prints the title and body instead, and does not commit.
#
# $1: The work directory, outside of the checkout.
# $2: The pull request title, which is also the commit message.
stencil_upgrade_commit() {
  local work_dir="$1" title="$2"
  local version

  version="$(stencil_upgrade_stencil_version)" || return 1
  local args=(--changes - --stencil-version "$version" --service-yaml service.yaml)
  if [[ -n ${STENCIL_UPGRADE_NOTE:-} ]]; then
    args+=(--note "$STENCIL_UPGRADE_NOTE")
  fi
  (devbase_cli stencil pr-body "${args[@]}") <"$work_dir/changes.json" >"$work_dir/body.md" || return 1

  if stencil_upgrade_is_dry_run; then
    info "dry run, so nothing is pushed. Title and body:"
    echo "$title"
    echo
    cat "$work_dir/body.md"
    return
  fi

  git add -A || return 1
  git commit -q -m "$title"
}

# stencil_upgrade_new_pr upgrades the modules on a new branch, then
# pushes it and opens a pull request.
#
# $1: The work directory, outside of the checkout.
# $2: The new branch.
# $3: The pull request title.
stencil_upgrade_new_pr() {
  local work_dir="$1" branch="$2" title="$3"
  local ls_remote_status=0 default_branch count force_pr changed tree_dirty=false

  # ls-remote exits with 2 when origin does not have the branch.
  git ls-remote --exit-code --heads origin "refs/heads/$branch" >/dev/null || ls_remote_status=$?
  if ((ls_remote_status == 0)); then
    error "$branch already exists on origin without an open pull request." \
      "Delete the branch, or open a pull request for it."
    return 1
  elif ((ls_remote_status != 2)); then
    error "could not list the branches on origin"
    return 1
  fi

  # The branch starts from the default branch, not from the checkout,
  # which can be on another branch in a manual run.
  default_branch="$(stencil_upgrade_default_branch)"
  stencil_upgrade_fetch "$default_branch" || return 1
  git switch -q --no-track -c "$branch" "origin/$default_branch" || return 1
  stencil_upgrade_stencil "$work_dir" "$default_branch" || return 1

  count="$(stencil_upgrade_version_change_count "$work_dir/changes.json")" || return 1
  force_pr="$(stencil_upgrade_force_pr "${STENCIL_UPGRADE_FORCE_PR:-false}" stencil.lock)" || return 1
  changed="$(git status --porcelain)" || return 1
  if [[ -n $changed ]]; then
    tree_dirty=true
  fi

  case "$(stencil_upgrade_decide "$count" "$force_pr" "$tree_dirty")" in
  stop-no-changes)
    info "no module changes, skipping"
    return 0
    ;;
  stop-empty)
    info "the pull request would be empty, skipping"
    return 0
    ;;
  esac

  stencil_upgrade_commit "$work_dir" "$title" || return 1
  if stencil_upgrade_is_dry_run; then
    return 0
  fi
  git push -u origin "$branch" || return 1
  run_gh pr create --title "$title" --head "$branch" --body-file "$work_dir/body.md"
}

# stencil_upgrade_existing_pr adds a commit with the newest upgrade to
# the branch of an open pull request, then updates the pull request. It
# skips the pull request when a person committed to its branch.
#
# $1: The work directory, outside of the checkout.
# $2: The branch of the open pull request.
# $3: The pull request title.
# $4: The author name of the bot.
# $5: The author email of the bot.
stencil_upgrade_existing_pr() {
  local work_dir="$1" branch="$2" title="$3" name="$4" email="$5"
  local default_branch other_commit count force_pr changed

  default_branch="$(stencil_upgrade_default_branch)"
  stencil_upgrade_fetch "$default_branch" "$branch" || return 1

  if ! other_commit="$(stencil_upgrade_only_bot_commits "origin/$default_branch" "origin/$branch" "$name" "$email")"; then
    # Without a commit, the check itself failed.
    if [[ -z $other_commit ]]; then
      return 1
    fi
    info "open PR has human commits, skipping: $other_commit"
    return 0
  fi

  git switch -q --no-track -C "$branch" "origin/$branch" || return 1
  # The plain git push needs an upstream. git switch --track works only
  # when a fetch refspec covers the branch, so the config is set directly.
  git config "branch.$branch.remote" origin || return 1
  git config "branch.$branch.merge" "refs/heads/$branch" || return 1
  stencil_upgrade_stencil "$work_dir" "$default_branch" || return 1

  count="$(stencil_upgrade_version_change_count "$work_dir/changes.json")" || return 1
  force_pr="$(stencil_upgrade_force_pr "${STENCIL_UPGRADE_FORCE_PR:-false}" stencil.lock)" || return 1
  if ((count == 0)) && [[ $force_pr != "true" ]]; then
    warn "no module version differs from $default_branch, so the open PR may now be empty and can be closed"
  fi

  changed="$(git status --porcelain)" || return 1
  if [[ -z $changed ]]; then
    info "the open PR is already current, skipping"
    return 0
  fi

  stencil_upgrade_commit "$work_dir" "$title" || return 1
  if stencil_upgrade_is_dry_run; then
    return 0
  fi
  # Never force-push: the branch only gains commits.
  git push || return 1
  run_gh pr edit "$branch" --title "$title" --body-file "$work_dir/body.md"
}

# stencil_upgrade_run runs the stencil-upgrade job in the current
# checkout. It opens a pull request that upgrades the Stencil modules, or
# adds the upgrade to the open pull request from an earlier run.
#
# It reads these environment variables:
# * STENCIL_UPGRADE_FORCE_PR: "true" opens a pull request when the
#   working tree changed, even without module version changes. Default:
#   "false".
# * STENCIL_UPGRADE_NOTE: Markdown shown at the top of the pull request
#   description. Default: empty, which shows no note.
# * STENCIL_UPGRADE_DRYRUN: "true" prints the title and body, and pushes
#   nothing. Default: "false".
#
# It returns 0 on success or when it stops on purpose.
stencil_upgrade_run() {
  local name email open_pr branch title work_dir status=0

  name="$(get_box_field ci.circleci.releaseUser.name)" || return 1
  email="$(get_box_field ci.circleci.releaseUser.email)" || return 1
  if [[ -z $name || -z $email ]]; then
    error "the box fields ci.circleci.releaseUser.name and ci.circleci.releaseUser.email must be set"
    return 1
  fi
  git config user.name "$name" || return 1
  git config user.email "$email" || return 1

  # A stencilbot/ pull request belongs to the dtss-tools stencilbot command.
  open_pr="$(stencil_upgrade_open_prs stencilbot/)" || return 1
  if [[ -n $open_pr ]]; then
    info "open stencilbot PR ${open_pr//$'\n'/ }, skipping"
    return 0
  fi
  open_pr="$(stencil_upgrade_open_prs stencil-upgrade/)" || return 1

  branch="$(stencil_upgrade_branch_name)" || return 1
  title="fix(stencil): upgrade modules (${branch#stencil-upgrade/})"
  work_dir="$(mktemp -d)" || return 1
  if [[ -n $open_pr ]]; then
    stencil_upgrade_existing_pr "$work_dir" "${open_pr%%$'\n'*}" "$title" "$name" "$email" || status=$?
  else
    stencil_upgrade_new_pr "$work_dir" "$branch" "$title" || status=$?
  fi
  rm -rf "$work_dir"
  return "$status"
}
