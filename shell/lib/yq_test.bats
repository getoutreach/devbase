#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load yq.sh

@test "check_unsupported_yq_flags allows a gojq-compatible bundled short flag (-cr)" {
  run check_unsupported_yq_flags -cr .name
  assert_success
}

@test "check_unsupported_yq_flags allows -y (--yaml-output), unlike -Y" {
  run check_unsupported_yq_flags -y .name
  assert_success
}

@test "check_unsupported_yq_flags allows --yaml-output" {
  run check_unsupported_yq_flags --yaml-output .name
  assert_success
}

@test "check_unsupported_yq_flags rejects -Y (--yaml-roundtrip)" {
  run check_unsupported_yq_flags -Y .name
  assert_failure
  assert_output --partial "yaml-roundtrip"
}

@test "check_unsupported_yq_flags rejects --yaml-roundtrip" {
  run check_unsupported_yq_flags --yaml-roundtrip .name
  assert_failure
  assert_output --partial "--yaml-roundtrip"
}

@test "check_unsupported_yq_flags rejects -i (--in-place) bundled with a valid flag" {
  run check_unsupported_yq_flags -ni .name
  assert_failure
  assert_output --partial "in-place"
}

@test "check_unsupported_yq_flags does not suggest a command when -i has no real file" {
  # Regression test: -ni .name has only a filter after stripping -i, no file
  # to build a temp-file-and-mv suggestion from. It must fall back to the
  # generic rejection instead of treating the filter as the file.
  run check_unsupported_yq_flags -ni .name
  assert_failure
  refute_output --partial "gojq --yaml-input"
}

@test "check_unsupported_yq_flags does not suggest a command when another unsupported flag rides along" {
  # Regression test: the suggested command must not itself contain a flag
  # gojq would also reject.
  run check_unsupported_yq_flags -i --width=80 '.name = "x"' file.yaml
  assert_failure
  refute_output --partial "gojq --yaml-input"
  refute_output --partial "--width"
}

@test "check_unsupported_yq_flags rejects --in-place" {
  run check_unsupported_yq_flags --in-place .name
  assert_failure
  assert_output --partial "--in-place"
}

@test "check_unsupported_yq_flags suggests a gojq equivalent for -i with a filter and file" {
  run check_unsupported_yq_flags -i '.name = "x"' file.yaml
  assert_failure
  assert_output --partial "gojq --yaml-input --yaml-output"
  assert_output --partial "file.yaml.tmp"
  assert_output --partial "mv file.yaml.tmp file.yaml"
}

@test "check_unsupported_yq_flags suggests a gojq equivalent for --in-place with a filter and file" {
  run check_unsupported_yq_flags --in-place '.name = "x"' file.yaml
  assert_failure
  assert_output --partial "gojq --yaml-input --yaml-output"
}

@test "check_unsupported_yq_flags suggests a gojq equivalent for -ni bundled with a filter and file" {
  run check_unsupported_yq_flags -ni '.name = "x"' file.yaml
  assert_failure
  assert_output --partial "gojq --yaml-input --yaml-output -n"
}

@test "check_unsupported_yq_flags rejects --width with an attached value" {
  run check_unsupported_yq_flags --width=80 .name
  assert_failure
  assert_output --partial "--width"
}

@test "check_unsupported_yq_flags rejects --max-expansion-factor" {
  run check_unsupported_yq_flags --max-expansion-factor 2048 .name
  assert_failure
  assert_output --partial "--max-expansion-factor"
}

@test "check_unsupported_yq_flags ignores flags after --" {
  run check_unsupported_yq_flags -n -- -Y-looking-filename.yaml
  assert_success
}

@test "check_unsupported_yq_flags allows a plain filter with no flags" {
  run check_unsupported_yq_flags .name file.yaml
  assert_success
}
