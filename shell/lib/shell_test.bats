#!/usr/bin/env bats
# Not needed for tests:
# - SC2155: Declare and assign separately to avoid masking return values.
# shellcheck disable=SC2155

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
