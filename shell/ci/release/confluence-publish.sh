#!/usr/bin/env bash

# This script publishes all markdown files with certain directives to confluence
# using said directives.
#
# Directives:
# <!-- Space: <confluence space key> --> (required)
# <!-- Parent: <title of parent page> --> (not required)
# <!-- Title: <title of page to publish to> --> (required)

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
GOBIN="${DIR}/../../gobin.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/shell.sh
source "${LIB_DIR}/shell.sh"

srcPath=$(get_repo_directory)
info "srcPath: ${srcPath}"

defaultBranch="$(git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@')"
info "defaultBranch: ${defaultBranch}"

while read -r file; do
  info "inspecting found markdown file: ${file}"
  if grep -Eq '^\s*<!--\s*Space:\s*\w+\s*-->\s*$' "${file}"; then
    info_sub "found space directive in ${file}:"
    info_sub "$(grep -E '^\s*<!--\s*Space:\s*\w+\s*-->\s*$' "${file}")" # Re-output this w/o quiet flag for debugging purposes.

    fullName="${srcPath}/${file}"
    fileDirName=$(dirname "$fullName")
    fileBaseName=$(basename "$fullName")
    updatedName="${fileBaseName}.updated.md"

    info_sub "dir: ${fileDirName}"
    info_sub "baseName: ${fileBaseName}"
    info_sub "updatedName: ${updatedName}"

    # Enter the directory of the md file to run command since the generated plots are relative path
    pushd "$fileDirName" >/dev/null || exit 1

    # Render
    "$GOBIN" "github.com/getoutreach/markdowntools/cmd/visualizemd@$(get_application_version "getoutreach/markdowntools/visualizemd")" \
      -umlusername internal_access_user -umlpassword "${UML_PASSWORD}" -umlserver https://rolling.mi.outreach-dev.com/api/internal/plantuml \
      -mermaidusername internal_access_user -mermaidpassword "${UML_PASSWORD}" -mermaidserver https://rolling.mi.outreach-dev.com/api/internal/mermaid \
      -u "${CONFLUENCE_USERNAME}" -p "${CONFLUENCE_API_TOKEN}" -no-add-parent -f "${fileBaseName}" -o "${updatedName}"

    # Add caveat to the end of the updated file.
    {
      echo ""
      echo "_________________"
      echo "Do not edit! This page is generated from a [markdown file in github](https://github.com/getoutreach/$(get_app_name)/blob/${defaultBranch}/${file}). Inline comments on this page are NOT preserved."
    } >>"$updatedName"

    # Push to confluence
    retry 5 5 "$GOBIN" "github.com/kovetskiy/mark@$(get_application_version "kovetskiy/mark")" \
      --minor-edit -u "${CONFLUENCE_USERNAME}" -p "${CONFLUENCE_API_TOKEN}" -b https://outreach-io.atlassian.net/wiki \
      -f "${updatedName}"

    # Delete generated file.
    if [[ -e $updatedName ]]; then
      rm "$updatedName" >/dev/null 2>&1 || true
    fi

    # Return to source directory.
    popd >/dev/null || exit 1
  else
    info_sub "no space directive found, skipping ${file}"
  fi
done < <(git ls-files -- '*.md')
