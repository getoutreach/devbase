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

@test "check_unsupported_yq_flags allows bundled -ry (r + yaml-output)" {
  run check_unsupported_yq_flags -ry .name
  assert_success
}

@test "check_unsupported_yq_flags rejects -S (--sort-keys)" {
  run check_unsupported_yq_flags -S .name
  assert_failure
  assert_output --partial "sort-keys"
}

@test "check_unsupported_yq_flags rejects --sort-keys" {
  run check_unsupported_yq_flags --sort-keys .name
  assert_failure
  assert_output --partial "--sort-keys"
}

@test "check_unsupported_yq_flags rejects -a (--ascii-output)" {
  run check_unsupported_yq_flags -a .name
  assert_failure
  assert_output --partial "ascii-output"
}

@test "check_unsupported_yq_flags rejects --ascii-output" {
  run check_unsupported_yq_flags --ascii-output .name
  assert_failure
  assert_output --partial "--ascii-output"
}

@test "normalize_yq_args translates bare -y to --yaml-output" {
  normalize_yq_args -y .name
  assert_equal "${NORMALIZED_YQ_ARGS[*]}" "--yaml-output .name"
}

@test "normalize_yq_args translates bundled -ry to -r --yaml-output" {
  normalize_yq_args -ry .name
  assert_equal "${NORMALIZED_YQ_ARGS[*]}" "-r --yaml-output .name"
}

@test "normalize_yq_args leaves flags after -- untouched" {
  normalize_yq_args -n -- -y-looking-filename.yaml
  assert_equal "${NORMALIZED_YQ_ARGS[*]}" "-n -- -y-looking-filename.yaml"
}

@test "check_unsupported_yq_flags suggests --yaml-output, not a raw -y, for bundled -yi" {
  # Regression test: -yi bundles -y (now translated to --yaml-output) with
  # -i (unsupported). The suggested command must contain the translated
  # long flag, not the untranslated short one.
  run check_unsupported_yq_flags -yi '.name = "x"' file.yaml
  assert_failure
  assert_output --partial "gojq --yaml-input --yaml-output"
  refute_output --partial "-yi"
}

@test "check_unsupported_yq_flags does not duplicate --yaml-output in the -yi suggestion" {
  # Regression test: normalize_yq_args already expands -yi's `y` into a
  # standalone --yaml-output; suggest_in_place_command must not add a
  # second one on top of the one it always prepends.
  run check_unsupported_yq_flags -yi '.name = "x"' file.yaml
  assert_failure
  refute_output --partial "--yaml-output --yaml-output"
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
