#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

# Loading kubeconform.sh runs its stub/bootstrap sourcing chain, but a
# sourced-guard (BASH_SOURCE != $0) returns before the side-effecting wrapper
# body, so only the pure functions are defined for these tests.
load kubeconform.sh

@test "kubeconform_k8s_sparse_dirs derives both standalone dirs from -kubernetes-version" {
  run kubeconform_k8s_sparse_dirs -ignore-missing-schemas -strict -kubernetes-version 1.25.16
  assert_success
  assert_line --index 0 "v1.25.16-standalone"
  assert_line --index 1 "v1.25.16-standalone-strict"
}

@test "kubeconform_k8s_sparse_dirs supports the --flag=value form" {
  run kubeconform_k8s_sparse_dirs -kubernetes-version=1.30.2
  assert_success
  assert_line --index 0 "v1.30.2-standalone"
  assert_line --index 1 "v1.30.2-standalone-strict"
}

@test "kubeconform_k8s_sparse_dirs prints nothing when no version is given" {
  run kubeconform_k8s_sparse_dirs -strict -summary
  assert_success
  assert_output ""
}
