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

# stub_flaky_gh makes gh fail until it has been called $1 times, then
# print v1.45.0. The call count is in $STUB_DIR/gh.count.
stub_flaky_gh() {
  echo "$1" >"$STUB_DIR/gh.succeed_at"
  cat >"$STUB_DIR/gh" <<'STUB'
#!/usr/bin/env bash
count="$(($(cat "$STUB_DIR/gh.count" 2>/dev/null || echo 0) + 1))"
echo "$count" >"$STUB_DIR/gh.count"
if [[ $count -lt $(cat "$STUB_DIR/gh.succeed_at") ]]; then
  echo "HTTP 502: Bad Gateway" >&2
  exit 1
fi
echo "v1.45.0"
STUB
  chmod +x "$STUB_DIR/gh"
}

@test "latest_github_release_version retries a failed lookup" {
  stub_flaky_gh 2
  stub_command sleep ""

  run --separate-stderr latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"
  [[ $stderr == *"retrying (1/3)"* ]]
  run cat "$STUB_DIR/gh.count"
  assert_output "2"
}

@test "latest_github_release_version fails after three failed lookups" {
  stub_flaky_gh 99
  stub_command sleep ""

  run latest_github_release_version getoutreach/stencil false
  assert_failure
  run cat "$STUB_DIR/gh.count"
  assert_output "3"
}

@test "latest_github_release_version doesn't retry when gh needs authentication" {
  stub_command gh "" 4
  stub_command sleep ""

  run latest_github_release_version getoutreach/stencil false
  assert_failure
  run grep -c "^gh release" "$STUB_CALLS_FILE"
  assert_output "1"
  assert_stub_not_called sleep
}

@test "latest_github_release_version fails without retrying when gh is missing" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  gh_installed() { return 1; }
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
