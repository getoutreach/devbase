#!/usr/bin/env bats
# Not needed for tests:
# - SC2155: Declare and assign separately to avoid masking return values.
# shellcheck disable=SC2155

bats_require_minimum_version 1.5.0

load shell.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  TMP_GITDIR=$(mktempdir devbase-shell-test-XXXXXX)
  git init --quiet "$TMP_GITDIR"
  pushd "$TMP_GITDIR" >/dev/null || exit 1
  git config user.name "Test User"
  git config user.email "test@example.com"

  mkdir -p .mise/tasks/foo
  echo -e "#!/usr/bin/env bash\n\necho 'Hello, World!'" >.mise/tasks/foo/bash_script
  echo -e "#!/usr/bin/env python3\n\nprint('Hello, World!')" >.mise/tasks/foo/python_script

  # Add and commit the script
  git add .mise/tasks/foo/bash_script
  git add .mise/tasks/foo/python_script
  git commit --quiet -m "Initial commit"

  echo -e "#!/usr/bin/env bash\n\necho 'Untracked file!'" >.mise/tasks/foo/untracked_bash_script
}

teardown() {
  popd >/dev/null || exit 1
  rm -rf "$TMP_GITDIR"
}

@test "find_files_with_shebang should find extension-less bash scripts in a git repo" {
  run find_files_with_shebang "/usr/bin/env bash" ".mise/tasks"
  assert_success
  assert_output - <<EOF
.mise/tasks/foo/bash_script
.mise/tasks/foo/untracked_bash_script
EOF
}

# flaky_echo fails until it has been called $1 times, then prints "ok".
flaky_echo() {
  local count
  count="$(($(cat "$TMP_GITDIR/flaky_calls" 2>/dev/null || echo 0) + 1))"
  echo "$count" >"$TMP_GITDIR/flaky_calls"
  if [[ $count -lt $1 ]]; then
    return 1
  fi
  echo "ok"
}

@test "retry keeps its notices out of the command's stdout" {
  run --separate-stderr retry 0 3 flaky_echo 2
  assert_success
  assert_output "ok"
  [[ $stderr == "RETRYING: flaky_echo (2/3)" ]]
}

@test "retry returns the command's exit code after the last attempt" {
  run retry 0 2 flaky_echo 5
  assert_failure
  run cat "$TMP_GITDIR/flaky_calls"
  assert_output "2"
}
