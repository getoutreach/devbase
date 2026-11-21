#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

load stencil_upgrade.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  TEST_DIR="$(mktempdir stencil-upgrade-XXXXXX)"
  setup_command_stubs
}

teardown() {
  # Some tests cd into TEST_DIR, which the next line removes.
  cd "$BATS_TEST_DIRNAME"
  teardown_command_stubs
  rm -rf "$TEST_DIR"
}

# commit_as NAME EMAIL SUBJECT makes an empty commit in the current repo.
commit_as() {
  git -c user.name="$1" -c user.email="$2" commit --allow-empty -q -m "$3"
}

# write_lock FILE VERSION... writes a stencil.lock with one module per
# version.
write_lock() {
  local file="$1" i=0
  shift
  {
    echo "modules:"
    for version in "$@"; do
      echo "  - name: example.com/module$((i++))"
      echo "    version: $version"
    done
  } >"$file"
}

# job_setup [REPO_NAME] makes a bare origin whose default branch is
# trunk, clones it into $TEST_DIR/work, and changes to that checkout.
# It also stubs the tools that stencil_upgrade_run uses:
# * mise: for `run stencil:upgrade`, it records MISE_TRUSTED_CONFIG_PATHS
#   in $STUB_DIR/mise.trusted, and copies $STUB_DIR/new.lock over
#   stencil.lock and $STUB_DIR/generated.txt into the checkout when those
#   files exist.
# * devbase_cli: `stencil lock-diff` copies its --old file to
#   $STUB_DIR/lock-diff.old and prints $STUB_DIR/changes.json;
#   `stencil pr-body` prints "PR body".
# * git: a wrapper that logs each call to $STUB_DIR/git.log, then runs git.
job_setup() {
  local name="${1:-example}" real_git
  real_git="$(command -v git)"
  # Ignore the git config of the host.
  export GIT_CONFIG_GLOBAL=/dev/null
  ORIGIN="$TEST_DIR/$name.git"
  git init -q --bare -b trunk "$ORIGIN"
  # A force push that rewrites history fails.
  git -C "$ORIGIN" config receive.denyNonFastForwards true
  git clone -q "$ORIGIN" "$TEST_DIR/work" 2>/dev/null
  cd "$TEST_DIR/work"
  git config user.name human
  git config user.email human@example.com
  echo "name: $name" >service.yaml
  write_lock stencil.lock v1.0.0
  git add -A
  git commit -q -m init
  git push -q -u origin trunk
  git remote set-head origin trunk

  echo "[]" >"$STUB_DIR/changes.json"
  stub_command date "2026-11-21"
  stub_command gh ""
  stub_command make ""
  stub_command stencil "stencil version v1.47.0"
  cat >"$STUB_DIR/mise" <<'EOF'
#!/usr/bin/env bash
echo "mise $*" >>"$STUB_CALLS_FILE"
if [[ $* == "run stencil:upgrade" ]]; then
  echo "$MISE_TRUSTED_CONFIG_PATHS" >"$STUB_DIR/mise.trusted"
  if [[ -f $STUB_DIR/new.lock ]]; then
    cp "$STUB_DIR/new.lock" stencil.lock
  fi
  if [[ -f $STUB_DIR/generated.txt ]]; then
    cp "$STUB_DIR/generated.txt" generated.txt
  fi
fi
EOF
  cat >"$STUB_DIR/git" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"$STUB_DIR/git.log"
exec "$real_git" "\$@"
EOF
  chmod +x "$STUB_DIR/mise" "$STUB_DIR/git"
  # The shell remembers the path of git, which would skip the wrapper.
  hash -r

  devbase_cli() {
    echo "devbase_cli $*" >>"$STUB_CALLS_FILE"
    case "$2" in
    lock-diff)
      cp "$4" "$STUB_DIR/lock-diff.old"
      cat "$STUB_DIR/changes.json"
      ;;
    pr-body)
      echo "PR body"
      ;;
    esac
  }

  get_box_field() {
    echo "get_box_field $*" >>"$STUB_CALLS_FILE"
    case "$1" in
    ci.circleci.releaseUser.name) echo bot ;;
    ci.circleci.releaseUser.email) echo bot@example.com ;;
    *) return 1 ;;
    esac
  }
}

# push_bot_branch BRANCH pushes BRANCH to origin with one bot commit that
# upgrades the module to v1.1.0, then deletes the local branch.
push_bot_branch() {
  git switch -q -c "$1"
  write_lock stencil.lock v1.1.0
  git add stencil.lock
  commit_as bot bot@example.com "fix(stencil): upgrade modules (2026-11-20)"
  git push -q origin "$1"
  git switch -q trunk
  git branch -q -D "$1"
}

# upgrade_to VERSION makes the stencil stub upgrade the module from
# v1.0.0 to VERSION.
upgrade_to() {
  write_lock "$STUB_DIR/new.lock" "$1"
  cat >"$STUB_DIR/changes.json" <<JSON
[{"module": "example.com/module0", "old": "v1.0.0", "new": "$1"}]
JSON
}

# run_job runs stencil_upgrade_run and logs only the git calls it makes.
run_job() {
  : >"$STUB_DIR/git.log"
  run stencil_upgrade_run
}

# assert_pushes LINE... asserts the git push calls of the job, as logged
# by the git wrapper. No arguments asserts that the job did not push.
assert_pushes() {
  run grep '^push' "$STUB_DIR/git.log"
  if (($# == 0)); then
    assert_output ""
  else
    assert_output "$(printf '%s\n' "$@")"
  fi
}

# assert_mise_ran_stencil COUNT asserts how many times the job ran the
# stencil:upgrade mise task.
assert_mise_ran_stencil() {
  run grep -c '^mise run stencil:upgrade$' "$STUB_CALLS_FILE"
  assert_output "$1"
}

@test "stencil_upgrade_branch_name should use the UTC date" {
  stub_command date "2026-11-21"

  run stencil_upgrade_branch_name
  assert_success
  assert_output "stencil-upgrade/2026-11-21"
  assert_stub_called date -u +%Y-%m-%d
}

@test "stencil_upgrade_open_prs should keep only exact prefix matches" {
  stub_command gh "$(printf '%s\n' \
    stencil-upgrade/2026-11-20 stencil-upgrade-other/x stencilbot/2026-11-19)"

  run stencil_upgrade_open_prs "stencil-upgrade/"
  assert_success
  assert_output "stencil-upgrade/2026-11-20"
  assert_stub_called gh pr list --state open --limit 200 \
    --search head:stencil-upgrade --json headRefName --jq .[].headRefName
}

@test "stencil_upgrade_open_prs should fail when gh fails" {
  stub_command gh "boom" 1

  run stencil_upgrade_open_prs "stencil-upgrade/"
  assert_failure
}

@test "stencil_upgrade_only_bot_commits should pass for bot commits only" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as human human@example.com base
  commit_as bot bot@example.com one
  commit_as bot bot@example.com two

  run stencil_upgrade_only_bot_commits main~2 main bot bot@example.com
  assert_success
  assert_output ""
}

@test "stencil_upgrade_only_bot_commits should fail and name a human commit" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as human human@example.com base
  commit_as bot bot@example.com one
  commit_as human human@example.com "human fix"

  run stencil_upgrade_only_bot_commits main~2 main bot bot@example.com
  assert_failure
  assert_output --partial "human fix"
  assert_output --partial "human <human@example.com>"
}

@test "stencil_upgrade_only_bot_commits should pass for an empty range" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as human human@example.com base

  run stencil_upgrade_only_bot_commits main main bot bot@example.com
  assert_success
  assert_output ""
}

@test "stencil_upgrade_only_bot_commits should fail for a bad ref" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as bot bot@example.com one

  run stencil_upgrade_only_bot_commits missing main bot bot@example.com
  assert_failure
}

@test "stencil_upgrade_force_pr should be true when the flag is true" {
  write_lock "$TEST_DIR/stencil.lock" v1.2.0

  run stencil_upgrade_force_pr true "$TEST_DIR/stencil.lock"
  assert_success
  assert_output "true"
}

@test "stencil_upgrade_force_pr should be true when a module is unstable" {
  write_lock "$TEST_DIR/stencil.lock" v1.2.0 unstable

  run stencil_upgrade_force_pr false "$TEST_DIR/stencil.lock"
  assert_success
  assert_output "true"
}

@test "stencil_upgrade_force_pr should be false otherwise" {
  write_lock "$TEST_DIR/stencil.lock" v1.2.0 rc local

  run stencil_upgrade_force_pr false "$TEST_DIR/stencil.lock"
  assert_success
  assert_output "false"
}

@test "stencil_upgrade_version_change_count should ignore unchanged unstable entries" {
  cat >"$TEST_DIR/changes.json" <<'JSON'
[
  {"module": "example.com/a", "old": "v1.0.0", "new": "v1.1.0"},
  {"module": "example.com/b", "old": "unstable", "new": "unstable", "unstable": true},
  {"module": "example.com/c", "old": "v2.0.0", "new": "unstable", "unstable": true}
]
JSON

  run stencil_upgrade_version_change_count "$TEST_DIR/changes.json"
  assert_success
  assert_output "2"
}

@test "stencil_upgrade_version_change_count should be 0 for no changes" {
  echo "[]" >"$TEST_DIR/changes.json"

  run stencil_upgrade_version_change_count "$TEST_DIR/changes.json"
  assert_success
  assert_output "0"
}

@test "stencil_upgrade_decide should follow the decision table" {
  local count force dirty want
  # count force_pr tree_dirty -> decision
  while read -r count force dirty want; do
    run stencil_upgrade_decide "$count" "$force" "$dirty"
    assert_success
    assert_output "$want"
  done <<'TABLE'
2 false false open
2 true false open
0 false false stop-no-changes
0 false true stop-no-changes
0 true true open
0 true false stop-empty
TABLE
}

@test "open stencilbot PR stops before running stencil" {
  job_setup
  stub_command gh "stencilbot/2026-11-19"

  run_job
  assert_success
  assert_output --partial "open stencilbot PR stencilbot/2026-11-19, skipping"
  assert_mise_ran_stencil 0
  assert_pushes
}

@test "new PR path commits, pushes and creates the PR" {
  job_setup
  upgrade_to v1.1.0
  local trunk
  trunk="$(git rev-parse trunk)"

  run_job
  assert_success
  assert_stub_called get_box_field ci.circleci.releaseUser.name
  assert_stub_called get_box_field ci.circleci.releaseUser.email
  assert_stub_not_called make
  assert_stub_called stencil --skip-update --version
  assert_stub_called devbase_cli stencil pr-body --changes - \
    --stencil-version v1.47.0 --service-yaml service.yaml
  # The diff compares the lock from before stencil ran.
  run cat "$STUB_DIR/lock-diff.old"
  assert_output --partial "version: v1.0.0"
  assert_pushes "push -u origin stencil-upgrade/2026-11-21"

  run git -C "$ORIGIN" log --format='%an <%ae> %s' trunk..stencil-upgrade/2026-11-21
  assert_output "bot <bot@example.com> fix(stencil): upgrade modules (2026-11-21)"
  run git -C "$ORIGIN" rev-parse stencil-upgrade/2026-11-21~1
  assert_output "$trunk"
  run git -C "$ORIGIN" show stencil-upgrade/2026-11-21:stencil.lock
  assert_output --partial "version: v1.1.0"

  run stub_argv gh
  assert_line --index 0 pr
  assert_line --index 1 create
  assert_line --index 2 --title
  assert_line --index 3 "fix(stencil): upgrade modules (2026-11-21)"
  assert_line --index 4 --head
  assert_line --index 5 stencil-upgrade/2026-11-21
  assert_line --index 6 --body-file
  assert_equal "${#lines[@]}" 8
}

@test "dry run prints the body and pushes nothing" {
  job_setup
  upgrade_to v1.1.0
  export STENCIL_UPGRADE_DRYRUN=true

  run_job
  assert_success
  assert_line "fix(stencil): upgrade modules (2026-11-21)"
  assert_line "PR body"
  assert_pushes
  run git log --oneline trunk..HEAD
  assert_output ""
  run git -C "$ORIGIN" branch --list 'stencil-upgrade/*'
  assert_output ""
  run grep -c '^gh pr \(create\|edit\)' "$STUB_CALLS_FILE"
  assert_output "0"
}

@test "no changes stops without a PR" {
  job_setup

  run_job
  assert_success
  assert_output --partial "no module changes, skipping"
  assert_mise_ran_stencil 1
  assert_pushes
  run grep -c '^gh pr create' "$STUB_CALLS_FILE"
  assert_output "0"
}

@test "forcePR opens a PR when only unstable modules exist and the tree changed" {
  job_setup
  write_lock stencil.lock unstable
  git commit -q -am "use unstable"
  git push -q origin trunk
  echo "new template output" >"$STUB_DIR/generated.txt"
  cat >"$STUB_DIR/changes.json" <<'JSON'
[{"module": "example.com/module0", "old": "unstable", "new": "unstable", "unstable": true}]
JSON
  # The unstable module implies forcePR.
  export STENCIL_UPGRADE_FORCE_PR=false

  run_job
  assert_success
  assert_pushes "push -u origin stencil-upgrade/2026-11-21"
  run git -C "$ORIGIN" show stencil-upgrade/2026-11-21:generated.txt
  assert_output "new template output"
  assert_stub_called gh pr create --title "fix(stencil): upgrade modules (2026-11-21)" \
    --head stencil-upgrade/2026-11-21 --body-file "$(stub_argv gh | tail -n 1)"
}

@test "forcePR never opens an empty PR" {
  job_setup
  export STENCIL_UPGRADE_FORCE_PR=true

  run_job
  assert_success
  assert_output --partial "the pull request would be empty, skipping"
  assert_pushes
  run grep -c '^gh pr create' "$STUB_CALLS_FILE"
  assert_output "0"
}

@test "stencil- repositories run make before mise" {
  job_setup stencil-example

  run_job
  assert_success
  run grep -o '^\(make\|mise run stencil:upgrade\)' "$STUB_CALLS_FILE"
  assert_output "$(printf '%s\n' make "mise run stencil:upgrade")"
}

@test "MISE_TRUSTED_CONFIG_PATHS is the checkout" {
  job_setup

  run_job
  assert_success
  run cat "$STUB_DIR/mise.trusted"
  assert_output "$(cd "$TEST_DIR/work" && pwd -P)"
}

@test "open bot-only PR gets an appended commit and gh pr edit" {
  job_setup
  push_bot_branch stencil-upgrade/2026-11-20
  stub_command gh "stencil-upgrade/2026-11-20"
  upgrade_to v1.2.0
  local before
  before="$(git -C "$ORIGIN" rev-parse stencil-upgrade/2026-11-20)"

  run_job
  assert_success
  assert_pushes "push"
  # The diff compares the lock on the default branch, not on the PR branch.
  run cat "$STUB_DIR/lock-diff.old"
  assert_output --partial "version: v1.0.0"

  run git -C "$ORIGIN" log --format='%an <%ae> %s' "$before..stencil-upgrade/2026-11-20"
  assert_output "bot <bot@example.com> fix(stencil): upgrade modules (2026-11-21)"
  run git -C "$ORIGIN" rev-parse stencil-upgrade/2026-11-20~1
  assert_output "$before"
  run git -C "$ORIGIN" show stencil-upgrade/2026-11-20:stencil.lock
  assert_output --partial "version: v1.2.0"

  run stub_argv gh
  assert_line --index 0 pr
  assert_line --index 1 edit
  assert_line --index 2 stencil-upgrade/2026-11-20
  assert_line --index 3 --title
  assert_line --index 4 "fix(stencil): upgrade modules (2026-11-21)"
  assert_line --index 5 --body-file
  assert_equal "${#lines[@]}" 7
}

@test "open PR that is already current is left alone" {
  job_setup
  push_bot_branch stencil-upgrade/2026-11-20
  stub_command gh "stencil-upgrade/2026-11-20"
  # The stencil stub writes the lock that the PR branch already has.
  write_lock "$STUB_DIR/new.lock" v1.1.0

  run_job
  assert_success
  assert_output --partial "the open PR is already current, skipping"
  assert_pushes
  run grep -c '^gh pr edit' "$STUB_CALLS_FILE"
  assert_output "0"
}

@test "open PR with a human commit is skipped without pushing" {
  job_setup
  push_bot_branch stencil-upgrade/2026-11-20
  git switch -q stencil-upgrade/2026-11-20
  commit_as human human@example.com "human fix"
  git push -q origin stencil-upgrade/2026-11-20
  git switch -q trunk
  stub_command gh "stencil-upgrade/2026-11-20"
  upgrade_to v1.2.0
  local before
  before="$(git -C "$ORIGIN" rev-parse stencil-upgrade/2026-11-20)"

  run_job
  assert_success
  assert_output --partial "open PR has human commits, skipping"
  assert_output --partial "human fix"
  assert_mise_ran_stencil 0
  assert_pushes
  run git -C "$ORIGIN" rev-parse stencil-upgrade/2026-11-20
  assert_output "$before"
}

@test "orphan branch fails without pushing" {
  job_setup
  push_bot_branch stencil-upgrade/2026-11-21
  upgrade_to v1.1.0

  run_job
  assert_failure
  assert_output --partial "stencil-upgrade/2026-11-21 already exists on origin"
  assert_mise_ran_stencil 0
  assert_pushes
  run grep -c '^gh pr create' "$STUB_CALLS_FILE"
  assert_output "0"
}
