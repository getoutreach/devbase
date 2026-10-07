#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load ../../../shell/lib/test_helper.sh

TASK="$BATS_TEST_DIRNAME/go"

# invoke_task runs the task in a fresh bash process with BASH_ENV unset.
# CI sets BASH_ENV to re-activate mise (eval "$(mise activate bash --shims)")
# on every new shell; without this, that re-prepends mise's real `go` shim
# ahead of the stub_command shim this file put on PATH.
invoke_task() {
  env -u BASH_ENV bash "$TASK"
}

setup() {
  REPOPATH=$(mktempdir devbase-build-go-XXXXXX)
  cd "$REPOPATH" || exit 1
  echo "name: myapp" >service.yaml

  setup_command_stubs
  stub_command go "" 0

  export BOXPATH
  BOXPATH=$(mktemp)
  cat >"$BOXPATH" <<EOF
config:
  org: testorg
EOF

  export OUTREACH_SECRETS_DIR
  OUTREACH_SECRETS_DIR=$(mktempdir devbase-build-go-secrets-XXXXXX)
  export OUTREACH_LOCAL_SECRETS_DIR
  OUTREACH_LOCAL_SECRETS_DIR=$(mktempdir devbase-build-go-home-XXXXXX)
}

teardown() {
  teardown_command_stubs
  rm -rf "$REPOPATH" "$BOXPATH" "$OUTREACH_SECRETS_DIR" "$OUTREACH_LOCAL_SECRETS_DIR"
}

@test "build/go builds ./plugin/... when both cmd and plugin exist" {
  mkdir -p cmd plugin

  run invoke_task
  assert_success

  run stub_argv go
  assert_output --partial "plugin/..."
  refute_output --partial "cmd/..."
}

@test "build/go builds ./cmd/... when only cmd exists" {
  mkdir -p cmd

  run invoke_task
  assert_success

  run stub_argv go
  assert_output --partial "cmd/..."
}

@test "build/go warns and exits 0 when neither cmd nor plugin exist" {
  run invoke_task
  assert_success
  assert_output --partial "no 'cmd' or 'plugin' directory found"
  assert_stub_not_called go
}

@test "build/go passes -trimpath by default and omits it when SKIP_TRIMPATH=true" {
  mkdir -p cmd

  run invoke_task
  assert_success
  run stub_argv go
  assert_output --partial "-trimpath"

  SKIP_TRIMPATH=true run invoke_task
  assert_success
  run stub_argv go
  refute_output --partial "-trimpath"
}

@test "build/go embeds the app version in ldflags" {
  mkdir -p cmd

  run invoke_task
  assert_success

  run stub_argv go
  assert_output --partial "github.com/getoutreach/gobox/pkg/app.Version=v0.0.0-dev"
}

@test "build/go fails with a clear error when box config has no org" {
  cat >"$BOXPATH" <<EOF
config: {}
EOF
  mkdir -p cmd

  run invoke_task
  assert_failure
  assert_output --partial "Could not determine the GitHub org"
  assert_stub_not_called go
}
