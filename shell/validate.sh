#!/usr/bin/env bash
set -e -o pipefail

# The linter is flaky in some environments so we allow it to be overridden.
# Also, if your editor already supports linting, you can make your tests run
# faster at little cost with:
# `LINTER=/bin/true make test``
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

LINTER="${LINTER:-"$DIR/golangci-lint.sh"}"
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"
GOBIN="$DIR/gobin.sh"
PROTOFMT=$("$DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_application_version "buf")")

info "Running linters"

# Run shellcheck on shell-scripts, only if installed.
info_sub "shellcheck"
# Make sure to ignore the monitoring/.terraform directory
# shellcheck disable=SC2038
if ! git ls-files '*.sh' | xargs -n40 "$SHELLCHECKPATH" -x -P SCRIPTDIR; then
  error "shellcheck failed on some files. Run 'make fmt' to fix."
  exit 1
fi

info_sub "shfmt"
if ! git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -s -d; then
  error "shfmt failed on some files. Run 'make fmt' to fix."
  exit 1
fi

# Validators to run when not using a library
if ! has_feature "library"; then
  if [[ -e deployments ]] && [[ -e monitoring ]]; then
    info_sub "terraform"
    for tfdir in deployments monitoring; do
      if ! "$DIR"/terraform.sh fmt -diff -check "$tfdir"; then
        error "terraform fmt $tfdir failed on some files. Run 'make fmt' to fix."
        exit 1
      fi
    done
  fi
fi

info_sub "protobuf"
if ! "$PROTOFMT" format --exit-code >/dev/null 2>&1; then
  error "protobuf format (buf format) failed on some files. Run 'make fmt' to fix."
  exit 1
fi

# GRPC client validation
if has_feature "grpc"; then
  if has_grpc_client "node"; then
    nodeSourceDir="$(get_repo_directory)/api/clients/node"

    pushd "$nodeSourceDir" >/dev/null 2>&1 || exit 1
    yarn_install_if_needed

    info_sub "prettier (node)"
    yarn pretty

    info_sub "eslint (node)"
    yarn lint
    popd >/dev/null 2>&1 || exit 1
  fi
fi

# get_time_ms returns the current time in milliseconds
# we use perl because we can't use the date command %N on macOS
get_time_ms() {
  perl -MTime::HiRes -e 'printf("%.0f\n",Time::HiRes::time()*1000)'
}

run_linter() {
  local linter_name="$1"
  shift
  local linter_bin="$1"
  shift
  local linter_args=("$@")

  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155
  local started_at="$(get_time_ms)"
  info_sub "  $linter_name"
  "$linter_bin" "${linter_args[@]}"
  exit_code=$?
  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155
  local finished_at="$(get_time_ms)"
  local duration="$((finished_at - started_at))"
  if [[ $exit_code -ne 0 ]]; then
    error "  $linter_name failed with exit code $exit_code"
    exit 1
  fi
  info_sub "    $linter_name finished in $(format_diff $duration) seconds"
}

format_diff() {
  local diff="$1"
  local seconds=$((diff / 1000))
  local ms=$((diff % 1000))
  printf "%d.%02ds" "$seconds" "$ms"
}

started_at="$(get_time_ms)"
for language in "$DIR/linters"/*.sh; do
  language="$(basename "$language")"
  language="${language%.sh}"

  info_sub "$language"

  # We use a sub-shell to prevent inheriting
  # the changes to functions/variables to the parent
  # (this) script
  (
    # Modified by the language file
    extensions=()

    # Why: Dynamic
    # shellcheck disable=SC1090
    source "$DIR/linters/$language.sh"

    matched=false
    for extension in "${extensions[@]}"; do
      # If we don't find any files with the extension, skip the run.
      if [[ "$(git ls-files "*.$extension" | wc -l | tr -d ' ')" -le 0 ]]; then
        continue
      fi
      matched=true
    done
    if [[ "$matched" == "false" ]]; then
      exit 0
    fi

    # Set by the language file
    linter
  )
done
finished_at="$(get_time_ms)"
duration="$((finished_at - started_at))"
info "Linters took $(format_diff $duration)s"
