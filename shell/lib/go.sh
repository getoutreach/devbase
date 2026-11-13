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
# a Stencil-managed go.mod must be: the go.stanza argument in service.yaml if
# set, otherwise the golang version in .tool-versions, which Stencil writes from
# versions.go. Prints nothing if neither exists.
#
# Dependencies: bootstrap.sh and mise.sh, which the script that sources this
# file has to source.
expected_go_mod_go_version() {
  local stanza
  stanza="$(stencil_arg go.stanza 2>/dev/null || true)"
  if [[ -n $stanza && $stanza != "null" ]]; then
    echo "$stanza"
    return
  fi

  version_from_toolversions "$(get_repo_directory)" golang 2>/dev/null || true
}

# go_mod_file_for_dir(module_dir)
#
# Prints the path of the go.mod file in module_dir, a directory relative to the
# repository root, in the form that stencil.lock uses: go.mod for the root, and
# <module_dir>/go.mod for any other directory.
go_mod_file_for_dir() {
  if [[ $1 == "." ]]; then
    echo "go.mod"
  else
    echo "$1/go.mod"
  fi
}

# go_mod_managed_by_stencil(module_dir)
#
# Succeeds if Stencil manages the go.mod file in module_dir, a directory
# relative to the repository root. This is the only place that pairs a module
# directory with the name that stencil.lock uses for its go.mod file.
#
# Dependencies: bootstrap.sh (managed_by_stencil), which the script that sources
# this file has to source.
go_mod_managed_by_stencil() {
  managed_by_stencil "$(go_mod_file_for_dir "$1")"
}

# go_mod_unmanaged_child_dirs
#
# Prints, one per line, the directories of the go.mod files that Stencil does not
# manage, except the root go.mod, which is the source of the sync and never a
# target.
go_mod_unmanaged_child_dirs() {
  local godir
  for godir in $(go_mod_dirs); do
    if [[ $godir == "." ]] || go_mod_managed_by_stencil "$godir"; then
      continue
    fi
    echo "$godir"
  done
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
  if ! go_mod_managed_by_stencil "$1"; then
    return 0
  fi

  local goMod
  goMod="$(go_mod_file_for_dir "$1")"

  local expected
  expected="$(expected_go_mod_go_version)"
  if [[ -z $expected ]]; then
    warn "Not checking the go directive of $goMod: found no Go version in go.stanza or .tool-versions"
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

# go_mod_sync_child(go_mod_file, root_go_mod, [modules])
#
# Syncs go_mod_file, a go.mod file that Stencil does not manage, with
# root_go_mod, which has to have a go directive: it sets the go directive to the
# one of root_go_mod, removes the toolchain directive, and sets the version of
# each module in modules, a space-separated list, to its version in root_go_mod.
# A module that root_go_mod does not require is skipped with a warning.
#
# Dependencies: sed.sh and logging.sh, which the script that sources this file
# has to source.
go_mod_sync_child() {
  local goMod="$1" rootGoMod="$2" modules="${3:-}"

  sed_replace '^go .*$' "go $(go_mod_go_version "$rootGoMod")" "$goMod"
  go_mod_drop_toolchain "$goMod"

  local module moduleVersion
  for module in $modules; do
    moduleVersion="$(grep "$module " "$rootGoMod" | awk '{print $2}' || true)"
    if [[ -z $moduleVersion ]]; then
      warn "Module $module not found in $rootGoMod, skipping"
      continue
    fi
    info "Syncing $module version in $goMod to $moduleVersion"
    sed_replace "\(\t$module\) .*" "\1 $moduleVersion" "$goMod"
  done
}

# check_tool_versions_no_golang_override
#
# Fails with 1 if the toolverOverride block of the repository's .tool-versions
# sets golang, because the Go version in use then differs from the one that
# Stencil writes to go.mod. Succeeds if Stencil does not manage the go.mod file
# in the repository root, or if there is no .tool-versions file.
#
# Dependencies: bootstrap.sh and logging.sh, which the script that sources this
# file has to source.
check_tool_versions_no_golang_override() {
  local toolVersions override
  toolVersions="$(get_repo_directory)/.tool-versions"
  if [[ ! -f $toolVersions ]]; then
    return 0
  fi

  override="$(awk '/<<Stencil::Block\(toolverOverride\)>>/ { inBlock = 1; next }
    /<<\/Stencil::Block>>/ { inBlock = 0 }
    inBlock && $1 == "golang" { print $2; exit }' "$toolVersions")"
  # This check comes last, because it needs a yq run and most repositories have
  # no override.
  if [[ -z $override ]] || ! go_mod_managed_by_stencil .; then
    return 0
  fi

  error "The toolverOverride block of .tool-versions sets golang $override. Remove it, set versions.go to $override in service.yaml, and run stencil."
  return 1
}
