#!/usr/bin/env bash

set -e

# The linter is flaky in some environments so we allow it to be overridden.
# Also, if your editor already supports linting, you can make your tests run
# faster at little cost with:
# `LINTER=/bin/true make test``
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LINTER="${LINTER:-"$DIR/golangci-lint.sh"}"

# If you want to run tests under a debugger, use this ENV var to specify the
# name of the package you wish to debug.  You can only debug one package at a
# time.
## PACKAGE_TO_DEBUG=

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# This comes from the Makefile. This makes the linter aware of it.
export TEST_TAGS=$TEST_TAGS

if [[ -z $TEST_PACKAGES ]]; then
  TEST_PACKAGES="./..."
fi

if [[ -n $CI ]]; then
  export GOFLAGS="${GOFLAGS} -mod=readonly"
fi

if [[ -n $GO_TEST_TIMEOUT ]]; then
  export TEST_FLAGS=${TEST_FLAGS:--timeout "$GO_TEST_TIMEOUT"}
fi

# Catches test dependencies by shuffling tests if the installed Go version supports it
currentver="$(go version | awk '{ print $3 }' | sed 's|go||')"
requiredver="1.17.0"
if [[ "$(printf '%s\n' "$requiredver" "$currentver" | sort -V | head -n1)" == "$requiredver" ]]; then
  TEST_FLAGS="$TEST_FLAGS -shuffle=on"
fi

# Although we highly encourage all projects to run test with a check for race coditions, teams can
# choose to temporarily disable this option
if [[ $RACE != "disabled" ]]; then
  TEST_FLAGS="$TEST_FLAGS -race"
fi

if [[ -n $WITH_COVERAGE || -n $CI ]]; then
  COVER_FLAGS=${COVER_FLAGS:- -coverpkg=./... -covermode=atomic -coverprofile=/tmp/coverage.out -cover}
fi

testInclude="$(get_repo_directory)/scripts/test.include.sh"
if [[ -e $testInclude ]]; then
  # Why: This is dynamic and can't be parsed
  # shellcheck disable=SC1090
  source "$testInclude"
fi

if [[ "$(git ls-files '*_test.go' | wc -l | tr -d ' ')" -gt 0 ]]; then
  info "Running go test ($TEST_TAGS)"

  format="dots-v2"
  if [[ -n $CI ]]; then
    format="pkgname"
  fi

  if [[ -n $TEST_OUTPUT_FORMAT ]]; then
    format="$TEST_OUTPUT_FORMAT"
  fi

  if [[ -n $BENCH_FLAGS ]]; then
    format="dots-v2"
  fi

  # Ensure this exists for tests results, just in case
  mkdir -p "bin"

  if [[ -n $PACKAGE_TO_DEBUG ]]; then
    TESTBIN=$(mktemp)

    # We build the binary ourselves, rather than doing it implicitly via the
    # Delve command line, because the Delve command line doesn't handle our
    # complex linker flags very well right now (v1.7.3).
    #
    # shellcheck disable=SC2086
    go test -c -o "${TESTBIN}" \
      $BENCH_FLAGS $COVER_FLAGS $TEST_FLAGS \
      -ldflags "-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing" -tags="$TEST_TAGS" "$PACKAGE_TO_DEBUG"

    # We pass along command line args to the executable so you can specify
    # `-test.run <regex>`, `-test.bench <regex>`, etc. if desired.  Try `-help`
    # for more information.
    "$DIR/gobin.sh" github.com/go-delve/delve/cmd/dlv@v"$(get_application_version "delve")" exec "${TESTBIN}" -- "$@"
  else
    exitCode=0
    # Why: We want these to split. For those wondering about "$@":
    # https://stackoverflow.com/questions/5720194/how-do-i-pass-on-script-arguments-that-contain-quotes-spaces
    # shellcheck disable=SC2086
    "$DIR/gobin.sh" gotest.tools/gotestsum@v"$(get_application_version "gotestsum")" \
      --junitfile "$(get_repo_directory)/bin/unit-tests.xml" --format "$format" -- $BENCH_FLAGS $COVER_FLAGS $TEST_FLAGS \
      -ldflags "-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing" -tags="$TEST_TAGS" \
      "$@" $TEST_PACKAGES || exitCode=$?

    if [[ -n $CI ]]; then
      # Move this to a temporary directoy so that we can control
      # what gets uploaded via the store_test_results call
      mkdir -p /tmp/test-results
      mv "$(get_repo_directory)/bin/unit-tests.xml" /tmp/test-results/
    fi

    exit $exitCode
  fi
fi
