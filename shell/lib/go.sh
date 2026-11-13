#!/usr/bin/env bash
#
# Go-related utility functions.
#
# Dependencies: golangci_lint_supports_go needs version.sh. This file does not
# source it, so the script that sources this file has to.

# Retrieve list of directories containing go.mod files in the repository.
# Use the IGNORED_GO_MOD_DIRS environment variable (space-separated
# directories) to skip specific directories.
go_mod_dirs() {
  git ls-files --cached --others --modified --exclude-standard go.mod '**/go.mod' | xargs dirname | while read -r gomodDir; do
    for ignored in ${IGNORED_GO_MOD_DIRS:-}; do
      if [[ $gomodDir == "$ignored" ]]; then
        continue 2
      fi
    done
    echo "$gomodDir"
  done | sort | uniq | xargs echo
}

# golangci_lint_built_with_go(version_output)
#
# Prints the Go version that built golangci-lint, from the first line of the
# output of `golangci-lint --version`, for example:
#   golangci-lint has version 2.9.0 built with go1.26.0 from abc1234 on 2026-02-10T18:00:00Z
# Prints nothing if that line has no such version, as with a development build
# of Go.
golangci_lint_built_with_go() {
  sed -n '/^golangci-lint has version /{s/^golangci-lint has version [^ ]* built with go\([0-9][0-9.]*\).*/\1/p;q;}' <<<"$1"
}

# golangci_lint_supports_go(go_version, built_with_go_version)
#
# Succeeds if the Go toolchain go_version is not newer than built_with_go_version,
# the Go that built golangci-lint. The patch versions and any prerelease suffix of
# go_version are ignored. Fails with 1 if the toolchain is newer, and with 2 if
# go_version has no major and minor version, as with a development build of Go.
golangci_lint_supports_go() {
  local goVersion="$1" builtWith="$2"
  if [[ ! $goVersion =~ ^([0-9]+)\.([0-9]+) ]]; then
    return 2
  fi
  has_minimum_version "${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.0" "$builtWith"
}

# go_mod_go_version(go_mod_file)
#
# Prints the version in the go directive of go_mod_file, or nothing if it has
# none.
go_mod_go_version() {
  awk '$1 == "go" { print $2; exit }' "$1"
}

# expected_go_mod_go_version
#
# Prints the Go version that Stencil provides, which is what the go directive of
# a Stencil-managed go.mod must be: the go.stanza argument, then the versions.go
# argument in service.yaml, then the golang version in .tool-versions. This is
# the order that go.mod.tpl in stencil-golang uses. Prints nothing if none exist.
#
# Dependencies: bootstrap.sh and mise.sh, which the script that sources this
# file has to source.
expected_go_mod_go_version() {
  local argName override
  for argName in go.stanza versions.go; do
    override="$(stencil_arg "$argName" 2>/dev/null || true)"
    if [[ -n $override && $override != "null" ]]; then
      echo "$override"
      return
    fi
  done

  version_from_toolversions "$(get_repo_directory)" golang 2>/dev/null || true
}

# check_go_mod_go_version(module_dir)
#
# Succeeds if the go directive of the go.mod in module_dir (relative to the
# repository root) is the version from expected_go_mod_go_version, for example
# after a dependency update raised it. Fails with 1 if it is not. Succeeds
# without checking if Stencil does not manage the go.mod, or if the expected
# version is not known.
#
# Dependencies: bootstrap.sh and logging.sh, which the script that sources this
# file has to source.
check_go_mod_go_version() {
  local moduleDir="$1" goMod="go.mod"
  if [[ $moduleDir != "." ]]; then
    goMod="$moduleDir/go.mod"
  fi

  if ! managed_by_stencil "$goMod"; then
    return 0
  fi

  local expected
  expected="$(expected_go_mod_go_version)"
  if [[ -z $expected ]]; then
    warn "Not checking the go directive of $goMod: found no Go version in service.yaml or .tool-versions"
    return 0
  fi

  local actual
  actual="$(go_mod_go_version "$(get_repo_directory)/$goMod")"
  if [[ $actual != "$expected" ]]; then
    error "The go directive in $goMod is '${actual:-none}', but Stencil provides Go $expected. Set it back, or change the Go version in service.yaml and run stencil."
    return 1
  fi
}

# go_mod_drop_toolchain(go_mod_file)
#
# Removes the toolchain directive from go_mod_file, and collapses each run of
# blank lines in the file into one. Does nothing if the file has no toolchain
# directive.
#
# Dependencies: sed.sh, which the script that sources this file has to source.
go_mod_drop_toolchain() {
  local goMod="$1"
  if ! grep -q '^toolchain ' "$goMod"; then
    return 0
  fi

  # The collapse is a second pass, because in one pass sed pairs the directive
  # with the blank line before it and prints it.
  sed_in_place -e '/^toolchain /d' "$goMod"
  sed_in_place -e '/^$/N;/\n$/D' "$goMod"
}
