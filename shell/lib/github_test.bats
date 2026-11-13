#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

load circleci.sh
load github.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  local MISE_CONFIG_DIR MISE_DATA_DIR MISE_STATE_DIR

  MISE_CONFIG_DIR="$(mktempdir mise-config-XXXXXX)"
  MISE_DATA_DIR="$(mktempdir mise-data-XXXXXX)"
  MISE_STATE_DIR="$(mktempdir mise-state-XXXXXX)"
  export MISE_CONFIG_DIR MISE_DATA_DIR MISE_STATE_DIR

  export MISE_GLOBAL_CONFIG_ROOT="$MISE_CONFIG_DIR"
  export MISE_GLOBAL_CONFIG_FILE="$MISE_CONFIG_DIR/global.toml"
  export MISE_OVERRIDE_CONFIG_FILENAMES="global.toml"
  {
    echo '[tools]'
    echo 'github-cli = "latest"'
    echo 'wait-for-gh-rate-limit = "1.1.1"'
  } >>"$MISE_GLOBAL_CONFIG_FILE"

  # Start every test outside CI, wherever the suite runs. Tests of the
  # CI-only rate limit check set CI themselves.
  unset CI

  setup_command_stubs
}

teardown() {
  rm -rf "$MISE_CONFIG_DIR"
  rm -rf "$MISE_DATA_DIR"
  rm -rf "$MISE_STATE_DIR"
  unset MISE_CONFIG_DIR
  unset MISE_DATA_DIR
  unset MISE_STATE_DIR
  unset MISE_GLOBAL_CONFIG_ROOT
  unset MISE_GLOBAL_CONFIG_FILE
  unset MISE_OVERRIDE_CONFIG_FILENAMES

  teardown_command_stubs
}

@test "separate mise install" {
  run mise doctor
  assert_output --partial "config: $MISE_CONFIG_DIR"
}

# stub_gh_pr JSON makes `gh pr view ... --jq FILTER` print FILTER applied
# to JSON, so the tests exercise the caller's real filter.
stub_gh_pr() {
  printf '%s' "$1" >"$BATS_TEST_TMPDIR/pr.json"
  cat >"$STUB_DIR/gh" <<EOF
#!/usr/bin/env bash
echo "gh \$*" >>"\$STUB_CALLS_FILE"
while [[ \$# -gt 0 ]]; do
  if [[ \$1 == --jq ]]; then
    exec jq -r "\$2" "$BATS_TEST_TMPDIR/pr.json"
  fi
  shift
done
EOF
  chmod +x "$STUB_DIR/gh"
}

@test "github_pr_is_draft_without_label is true for a draft without the label" {
  local pr="https://github.com/getoutreach/example/pull/7"
  stub_gh_pr '{"isDraft": true, "labels": [{"name": "bug"}]}'

  run github_pr_is_draft_without_label "$pr" run-e2e
  assert_success

  run grep -F "gh pr view $pr --json isDraft,labels --jq " "$STUB_CALLS_FILE"
  assert_success
}

@test "github_pr_is_draft_without_label is false for a draft with the label" {
  stub_gh_pr '{"isDraft": true, "labels": [{"name": "bug"}, {"name": "run-e2e"}]}'

  run github_pr_is_draft_without_label https://github.com/getoutreach/example/pull/7 run-e2e
  assert_failure
}

@test "github_pr_is_draft_without_label is false for a pull request that is ready for review" {
  stub_gh_pr '{"isDraft": false, "labels": []}'

  run github_pr_is_draft_without_label https://github.com/getoutreach/example/pull/7 run-e2e
  assert_failure
}

@test "github_pr_is_draft_without_label is false when the pull request can't be read" {
  stub_command gh "HTTP 502" 1

  run github_pr_is_draft_without_label https://github.com/getoutreach/example/pull/7 run-e2e
  assert_failure
  assert_output --partial "Could not read https://github.com/getoutreach/example/pull/7"
}

@test "latest_github_release_version asks gh to exclude pre-releases when they are not wanted" {
  stub_command gh "v1.45.0"

  run latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"

  run stub_argv gh
  assert_line "getoutreach/stencil"
  assert_line "--exclude-drafts"
  assert_line "--exclude-pre-releases"
}

@test "latest_github_release_version takes the newest release of any kind when pre-releases are wanted" {
  stub_command gh "v1.45.0"

  run latest_github_release_version getoutreach/stencil true
  assert_success
  assert_output "v1.45.0"

  run stub_argv gh
  assert_line "--exclude-drafts"
  refute_line "--exclude-pre-releases"
}

@test "latest_github_release_version waits for the GraphQL rate limit in CI" {
  stub_command gh "v1.45.0"
  stub_command wait-for-gh-rate-limit ""

  CI=true GITHUB_TOKEN=test-token run latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"

  run stub_argv wait-for-gh-rate-limit
  assert_output "graphql"
  run head -n 1 "$STUB_CALLS_FILE"
  assert_output "wait-for-gh-rate-limit graphql"
}

@test "latest_github_release_version uses gh's token for the rate limit check when GITHUB_TOKEN is unset" {
  stub_command gh "v1.45.0"
  stub_command wait-for-gh-rate-limit ""

  CI=true GITHUB_TOKEN='' run latest_github_release_version getoutreach/stencil false
  assert_success

  run grep -c "^gh auth token" "$STUB_CALLS_FILE"
  assert_output "1"
}

@test "latest_github_release_version continues when the rate limit check fails" {
  stub_command gh "v1.45.0"
  stub_command wait-for-gh-rate-limit "" 1

  CI=true GITHUB_TOKEN=test-token run --separate-stderr latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"
  [[ $stderr == *"Could not check the GitHub GraphQL rate limit"* ]]
}

@test "latest_github_release_version doesn't wait for the rate limit outside CI" {
  stub_command gh "v1.45.0"
  stub_command wait-for-gh-rate-limit ""

  CI='' run latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"
  assert_stub_not_called wait-for-gh-rate-limit
}

@test "latest_github_release_version retries a failed lookup" {
  stub_command gh "v1.45.0" "1 0"
  stub_command sleep ""

  run --separate-stderr latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"
  [[ $stderr == *"retrying (1/3)"* ]]
  assert_stub_call_count gh 2
}

@test "latest_github_release_version fails after three failed lookups" {
  stub_command gh "" 1
  stub_command sleep ""

  run latest_github_release_version getoutreach/stencil false
  assert_failure
  assert_stub_call_count gh 3
}

@test "latest_github_release_version doesn't retry when gh needs authentication" {
  stub_command gh "" 4
  stub_command sleep ""

  run latest_github_release_version getoutreach/stencil false
  assert_failure
  assert_stub_call_count gh 1
  assert_stub_not_called sleep
}

@test "latest_github_release_version fails without retrying when gh is missing" {
  stub_function gh_installed 1
  stub_command sleep ""

  run latest_github_release_version getoutreach/stencil false
  assert_failure
  assert_output --partial "gh is required to look up the latest release of getoutreach/stencil"
  assert_stub_not_called sleep
}

@test "install_latest_github_release fails when there is no release to install" {
  stub_command gh ""

  run install_latest_github_release getoutreach/stencil false stencil
  assert_failure
  assert_output --partial "Failed to determine version for getoutreach/stencil"
}

@test "install_latest_github_release should be able to download and install the latest release of a repo" {
  if circleci_pr_is_fork; then
    skip "Skipping test in fork PR, no GitHub token available to utilize gh."
  fi
  install_latest_github_release getoutreach/stencil false stencil

  # We expect the stencil binary to be installed in the install dir.
  assert mise which stencil

  run "$(mise which stencil)" --version
  assert_success
}

@test "install_latest_github_release should be able to download and install the latest pre-release of a repo" {
  if circleci_pr_is_fork; then
    skip "Skipping test in fork PR, no GitHub token available to utilize gh."
  fi

  # The newest release is the stable one between a release and the next
  # commit to the repo, so a pre-release is not always available.
  local tag
  tag="$(latest_github_release_version getoutreach/stencil true)"
  if [[ ! $tag =~ (rc|unstable) ]]; then
    skip "Newest getoutreach/stencil release ($tag) is stable, not a pre-release."
  fi

  install_latest_github_release getoutreach/stencil true stencil

  # We expect the stencil binary to be installed in the install dir.
  assert mise which stencil

  run "$(mise which stencil)" --version
  assert_success
  assert_output --regexp "(rc|unstable)"
}
