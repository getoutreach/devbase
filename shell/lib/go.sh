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
