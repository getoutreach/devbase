#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load secrets.sh
load test_helper.sh

setup() {
  REPOPATH=$(mktempdir devbase-lib-secrets-XXXXXX)
  cd "$REPOPATH" || exit 1
  echo "name: myapp" >service.yaml

  OUTREACH_SECRETS_DIR=$(mktempdir devbase-lib-secrets-run-XXXXXX)
  OUTREACH_LOCAL_SECRETS_DIR=$(mktempdir devbase-lib-secrets-home-XXXXXX)
}

teardown() {
  rm -rf "$REPOPATH" "$OUTREACH_SECRETS_DIR" "$OUTREACH_LOCAL_SECRETS_DIR"
}

@test "read_local_secret finds a secret under OUTREACH_SECRETS_DIR" {
  mkdir -p "$OUTREACH_SECRETS_DIR/honeycomb"
  printf 'abc123' >"$OUTREACH_SECRETS_DIR/honeycomb/apiKey"

  run read_local_secret "honeycomb/apiKey"
  assert_success
  assert_output "abc123"
}

@test "read_local_secret falls back to OUTREACH_LOCAL_SECRETS_DIR/<appName>" {
  mkdir -p "$OUTREACH_LOCAL_SECRETS_DIR/myapp/telefork/api-keys"
  printf 'xyz789' >"$OUTREACH_LOCAL_SECRETS_DIR/myapp/telefork/api-keys/default"

  run read_local_secret "telefork/api-keys/default"
  assert_success
  assert_output "xyz789"
}

@test "read_local_secret prefers OUTREACH_SECRETS_DIR over the home fallback" {
  mkdir -p "$OUTREACH_SECRETS_DIR/honeycomb" "$OUTREACH_LOCAL_SECRETS_DIR/myapp/honeycomb"
  printf 'from-run-secrets' >"$OUTREACH_SECRETS_DIR/honeycomb/apiKey"
  printf 'from-home' >"$OUTREACH_LOCAL_SECRETS_DIR/myapp/honeycomb/apiKey"

  run read_local_secret "honeycomb/apiKey"
  assert_output "from-run-secrets"
}

@test "read_local_secret fails when the secret isn't found anywhere" {
  run read_local_secret "honeycomb/apiKey"
  assert_failure
  assert_output ""
}

@test "read_local_secret accepts an explicit appName, skipping get_app_name" {
  mkdir -p "$OUTREACH_LOCAL_SECRETS_DIR/otherapp/honeycomb"
  printf 'explicit-app' >"$OUTREACH_LOCAL_SECRETS_DIR/otherapp/honeycomb/apiKey"

  run read_local_secret "honeycomb/apiKey" "otherapp"
  assert_success
  assert_output "explicit-app"
}
