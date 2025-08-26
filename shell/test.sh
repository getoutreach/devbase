#!/usr/bin/env bash
# Runs tests for the current project.
set -euo pipefail

# DIR is the directory that this scripts lives in.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# CI denotes if we're in CI or not. When running in CI, certain
# environment variables below behaviour may differ. Check each
# environment variables' documentation for more information.
CI="${CI:-}"

# TEST_FLAGS is an array of flags to pass to `go test`. TEST_FLAGS must
# be a string value. It will be split on spaces.
#
# Why: We need to support importing flags from a string variable, e.g.,
# CircleCI in-line environment variables or contexts.
# shellcheck disable=SC2206
TEST_FLAGS=(${TEST_FLAGS:-})

# TEST_TAGS is an array of tags to pass to `go test -tags`. TEST_TAGS
# must be a string value. It will be split on spaces.
#
# Why: We need to support importing flags from a string variable, e.g.,
# CircleCI in-line environment variables or contexts.
# shellcheck disable=SC2206
TEST_TAGS=(${TEST_TAGS:-})

# TEST_PACKAGES is the packages to test. Defaults to "./...".
# TEST_PACKAGES must be a string value. It will be split on spaces.
#
# If PACKAGE_TO_DEBUG is set, TEST_PACKAGES will be ignored.
#
# Why: We need to support importing flags from a string variable, e.g.,
# CircleCI in-line environment variables or contexts.
# shellcheck disable=SC2206
TEST_PACKAGES=(${TEST_PACKAGES:-"./..."})

# PACKAGE_TO_DEBUG is a package that will be built and run under the
# delve debugger. When set, TEST_PACKAGE will have no effect.
PACKAGE_TO_DEBUG="${PACKAGE_TO_DEBUG:-}"

# BENCH_FLAGS is an array of flags to pass to `go test -bench`.
# BENCH_FLAGS must be a string value. It will be split on spaces.
#
# Why: We need to support importing flags from a string variable, e.g.,
# CircleCI in-line environment variables or contexts.
# shellcheck disable=SC2206
BENCH_FLAGS=(${BENCH_FLAGS:-})

# GO_FLAGS is an array of flags to pass to any go command being ran.
# GO_FLAGS must be a string value. It will be split on spaces.
#
# Why: We need to support importing flags from a string variable, e.g.,
# CircleCI in-line environment variables or contexts.
# shellcheck disable=SC2206
GO_FLAGS=(${GO_FLAGS:-})

# COVER_FLAGS is an array of flags to pass to `go test -cover`.
# COVER_FLAGS must be a string value. It will be split on spaces.
#
# Why: We need to support importing flags from a string variable, e.g.,
# CircleCI in-line environment variables or contexts.
# shellcheck disable=SC2206
COVER_FLAGS=(${COVER_FLAGS:-})

# WITH_COVERAGE is a boolean flag that enables coverage reporting. If
# set at all, a coverage file will be generated during the test. It
# will be outputted to /tmp/coverage.out. When running in CI, this is
# always enabled.
WITH_COVERAGE="${WITH_COVERAGE:-}"

# GO_TEST_TIMEOUT is the timeout to pass to `go test -timeout`. If not
# set, no timeout will be passed.
GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-}"

# RACE determines if 'go test -race' should be enabled or not. If set to
# 'disabled', race condition testing will not be enabled and the flag
# will not be passed to 'go test'. Defaults to 'enabled'.
RACE="${RACE:-enabled}"

# SHUFFFLE determines if 'go test -shuffle' should be enabled or not. If set to
# 'disabled', test order randomization will not be enabled and the flag will
# not be passed to 'go test'. Note that `-shuffle` is incompatible with test
# caching, so disabling it can result in a substantial speedup in development.
# Defaults to 'enabled' if the version of Go supports it (>=1.17).
SHUFFLE="${SHUFFLE:-enabled}"

# TEST_OUTPUT_FORMAT is the format to pass to gotestsum. If not set,
# the default value of "dots-v2" will be used. If CI is set, the default
# value is "pkgname". If 'TEST_FLAGS' contains '-v', the default value
# is "standard-verbose".
TEST_OUTPUT_FORMAT="${TEST_OUTPUT_FORMAT:-}"

if [[ -n $CI ]]; then
  GOFLAGS+=(-mod=readonly)
  WITH_COVERAGE="true"

  # Ensure that all processes recieve the value of GOFLAGS.
  export GOFLAGS
fi

# If GO_TEST_TIMEOUT is set, we pass it to `go test` as a timeout.
if [[ -n $GO_TEST_TIMEOUT ]]; then
  TEST_FLAGS+=(-timeout "$GO_TEST_TIMEOUT")
fi

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# REPODIR is the base directory of the repository.
REPODIR=$(get_repo_directory)

# Catches test dependencies by shuffling tests if the installed Go version supports it
currentver="$(go version | awk '{ print $3 }' | sed 's|go||')"
requiredver="1.17.0"
if [[ "$(printf '%s\n' "$requiredver" "$currentver" | sort -V | head -n1)" == "$requiredver" && $SHUFFLE != "disabled" ]]; then
  TEST_FLAGS+=(-shuffle=on)
fi

# Although we highly encourage all projects to run test with a check for race coditions, teams can
# choose to temporarily disable this option
if [[ $RACE != "disabled" ]]; then
  TEST_FLAGS+=(-race)
fi

# If WITH_COVERAGE is set, we pass flags to 'go test' to enable coverage
# file generation.
if [[ -n $WITH_COVERAGE ]]; then
  COVER_FLAGS+=(-coverpkg=./... -covermode=atomic -coverprofile=/tmp/coverage.out -cover)
fi

testInclude="$(get_repo_directory)/scripts/test.include.sh"
if [[ -e $testInclude ]]; then
  info "Running project specific test file: $testInclude"
  # Why: This is dynamic and can't be parsed
  # shellcheck disable=SC1090
  source "$testInclude"
fi

if [[ "$(git ls-files '*_test.go' | wc -l | tr -d ' ')" -gt 0 ]]; then
  info "Running go test (${TEST_TAGS[*]})"

  format="dots-v2"
  if [[ -n $CI ]]; then
    # When in CI, always use the pkgname format because it's easier to
    # read.
    format="pkgname"
  fi

  # If TEST_OUTPUT_FORMAT is set, we use it instead of the default value.
  if [[ -n $TEST_OUTPUT_FORMAT ]]; then
    format="$TEST_OUTPUT_FORMAT"
  fi

  for flag in "${TEST_FLAGS[@]}"; do
    # If we have -v passed as a test flag, we use the standard-verbose
    # format which is equal to 'go test -v'. Without this, we would
    # silently ignore the flag.
    if [[ $flag == "-v" ]]; then
      format="standard-verbose"
    fi
  done

  # Ensure this exists for tests results, just in case
  mkdir -p "bin"

  # Convert TEST_TAGS into the expected format for `go test -tags`,
  # which is a comma-separated string.
  test_tags_string=$(
    IFS=","
    echo "${TEST_TAGS[*]}"
  )

  if [[ -n $PACKAGE_TO_DEBUG ]]; then
    TESTBIN=$(mktemp)

    # We build the binary ourselves, rather than doing it implicitly via the
    # Delve command line, because the Delve command line doesn't handle our
    # complex linker flags very well right now (v1.7.3).
    go test -c -o "${TESTBIN}" \
      "${BENCH_FLAGS[@]}" "${COVER_FLAGS[@]}" "${TEST_FLAGS[@]}" \
      -ldflags "-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing" \
      -tags="$test_tags_string" "$PACKAGE_TO_DEBUG"

    # We pass along command line args to the executable so you can specify
    # `-test.run <regex>`, `-test.bench <regex>`, etc. if desired.  Try `-help`
    # for more information.
    exec "$DIR/gobin.sh" github.com/go-delve/delve/cmd/dlv@v"$(get_application_version "delve")" exec "${TESTBIN}" -- "$@"
  else
    exitCode=0

    GOTESTSUMPATH=$("$DIR/gobin.sh" -p gotest.tools/gotestsum@v"$(get_application_version "gotestsum")")
    (
      set -x
      "$GOTESTSUMPATH" --junitfile "$REPODIR/bin/unit-tests.xml" --format "$format" -- \
        "${BENCH_FLAGS[@]}" "${COVER_FLAGS[@]}" "${TEST_FLAGS[@]}" \
        -ldflags "-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing" \
        -tags="$test_tags_string" "$@" "${TEST_PACKAGES[@]}"
    ) || exitCode=$?

    if [[ -n $CI ]]; then
      # Move this to a temporary directory so that we can control
      # what gets uploaded via the store_test_results call
      mkdir -p /tmp/test-results
      mv "$REPODIR/bin/unit-tests.xml" /tmp/test-results/
    fi

    exit $exitCode
  fi
fi
