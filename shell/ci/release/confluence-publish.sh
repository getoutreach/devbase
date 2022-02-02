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

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

srcPath=$DIR
info "srcPath: ${DIR}"

tag=$(eval "git describe --tags --abbrev=0")
findCmd="git diff-tree --no-commit-id --name-status -r ${tag} HEAD | grep -e '^[A|M].*\.md$' | cut -f2"

info "findCmd: ${findCmd}"

defaultBranch="$(git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@')"

info "defaultBranch: ${defaultBranch}"

retry() {
  max_attempts="$1"
  shift
  attempt_num=1
  until "$@"; do
    info "retry attempt ${attempt_num}"

    if [ "$((attempt_num))" -eq "$((max_attempts))" ]; then
      info_sub "Attempt $attempt_num failed and there are no more attempts left!"
      exit 1
    else
      info_sub "Attempt $attempt_num failed! Trying again in $attempt_num seconds..."
      attempt_num=$((attempt_num + 1))
      sleep "$(attempt_num)"
    fi
  done
}

for file in $(eval "${findCmd}"); do
  info "inspecting found markdown file: ${file}"
  if grep -q '^\s*<!--\s*Space:\s*.+\s*-->\s*$' "${file}"; then
    info "found space directive in ${file}:\n---"
    grep '^\s*<!--\s*Space:\s*.+\s*-->\s*$' "${file}" # Re-output this w/o quiet flag for debugging purposes.
    info "---"

    {
      echo ""
      echo "_________________"
      echo "Do not edit! File is auto-generated from [github](https://github.com/getoutreach/${get_app_name}/blob/${defaultBranch}/${file})."
    } >> "$file"

    fullname="${srcPath}/${file}"
    filedirname=$(dirname "$fullname")
    filebasename=$(basename "$fullname")
    updatedname="${filebasename}.updated.md"

    info_sub "dir: ${filedirname}"
    info_sub "basename: ${filebasename}"
    info_sub "updatedname: ${updatedname}"

    # Enter the directory of the md file to run command since the generated plots are relative path
    cd "$filedirname" || exit

    # Render
    "$srcPath"/scripts/shell-wrapper.sh gobin.sh github.com/getoutreach/markdowntools/cmd/visualizemd@v0.0.24 -umlusername internal_access_user -umlpassword "$UML_PASSWORD" -umlserver https://rolling.mi.outreach-dev.com/api/internal/plantuml -u "$CONFLUENCE_USERNAME" -p "$CONFLUENCE_API_TOKEN" -f "$filebasename" >"$updatedname"
    cat "$updatedname"

    # Push to confluence
    retry 10 "$srcPath"/scripts/shell-wrapper.sh gobin.sh github.com/kovetskiy/mark@6.5 --minor-edit -u "$CONFLUENCE_USERNAME" -p "$CONFLUENCE_API_TOKEN" -b https://outreach-io.atlassian.net/wiki -f "$updatedname" --debug

    # Return to source directory.
    cd "$srcPath" || exit
  fi
done
