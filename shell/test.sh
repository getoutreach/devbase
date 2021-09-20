#!/usr/bin/env bash

set -e

# The linter is flaky in some environments so we allow it to be overridden.
# Also, if your editor already supports linting, you can make your tests run
# faster at little cost with:
# `LINTER=/bin/true make test``
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LINTER="${LINTER:-"$DIR/golangci-lint.sh"}"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

if [[ -n $CI ]]; then
  TEST_TAGS=${TEST_TAGS:-or_test,or_int}
  export GOFLAGS="${GOFLAGS} -mod=readonly"
else
  TEST_TAGS=${TEST_TAGS:-or_test}
fi
export TEST_TAGS

if [[ $TEST_TAGS == *"or_int"* ]]; then
  BENCH_FLAGS=${BENCH_FLAGS:--bench=^Bench -benchtime=1x}
fi

if [[ -n $GO_TEST_TIMEOUT ]]; then
  TEST_FLAGS=${TEST_FLAGS:--timeout "$GO_TEST_TIMEOUT"}
fi

if [[ -n $WITH_COVERAGE || -n $CI ]]; then
  COVER_FLAGS=${COVER_FLAGS:- -covermode=atomic -coverprofile=/tmp/coverage.out -cover}
fi

if ! grep or_e2e <<<"$TEST_TAGS" >/dev/null 2>&1; then
  info "Verifying go.{mod,sum} files are up to date"
  go mod tidy

  # We only ever error on this in CI, since it's updated when we run the above...
  # Eventually we can do `go mod tidy -check` or something else:
  # https://github.com/golang/go/issues/27005
  if [[ -n $CI ]]; then
    git diff --exit-code go.{mod,sum} || fatal "go.{mod,sum} are out of date, please run 'go mod tidy' and commit the result"
  fi

  # Perform linting and format validations
  if [[ -n $SKIP_VALIDATE ]]; then
    info "Skipping linting and format validations"
  else
    OSS=$(OSS) "$DIR/validate.sh"
  fi
fi

if [[ -z $CI && $TEST_TAGS == *"or_int"* ]]; then
  # shellcheck disable=SC2034
  cleanup="true"

  if has_resource "postgres"; then
    info_sub "creating postgres container"
    pgID=$(docker run -itd --rm -p 5432:5432 -e "POSTGRES_DB=$(get_app_name)" -e POSTGRES_HOST_AUTH_METHOD=trust "gcr.io/outreach-docker/postgres:$(get_resource_version "postgres")")
    cleanup="$cleanup; docker stop $pgID"
    # shellcheck disable=SC2064
    trap "$cleanup" EXIT INT TERM
    sleep 10
  fi

  if has_resource "mysql"; then
    info_sub "creating mysql container"
    mysqlID=$(docker run -itd --rm -p 3306:3306 -e "MYSQL_DATABASE=$(get_app_name)" -e MYSQL_ROOT_PASSWORD=root "gcr.io/outreach-docker/mysql:$(get_resource_version "mysql")")
    cleanup="$cleanup; docker stop $mysqlID"
    # shellcheck disable=SC2064
    trap "$cleanup" EXIT INT TERM
    sleep 10
  fi

  if has_resource "redis"; then
    info_sub "creating redis container"
    redisID=$(docker run -itd --rm -p 6379:6379 "gcr.io/outreach-docker/redis:$(get_resource_version "redis")")
    cleanup="$cleanup; docker stop $redisID"
    # shellcheck disable=SC2064
    trap "$cleanup" EXIT INT TERM
    sleep 10
  fi

  if has_resource "kafka"; then
    info_sub "creating kafka container"
    kafkaID=$(docker run -itd --rm -p 9092:9092 --env ADV_HOST=localhost --env BROKER_PORT=9092 "gcr.io/outreach-docker/kafka:$(get_resource_version "kafka")")
    cleanup="$cleanup; docker stop $kafkaID"
    # shellcheck disable=SC2064
    trap "$cleanup" EXIT INT TERM
    sleep 10
  fi

  if has_resource "s3"; then
    info_sub "creating minio container (aws)"
    minioID=$(docker run -itd --rm -p 9000:9000 --env MINIO_ACCESS_KEY=fake_key --env MINIO_SECRET_KEY=fake_secret "gcr.io/outreach-docker/minio:$(get_resource_version "s3")" server /data)
    cleanup="$cleanup; docker stop $minioID"
    # shellcheck disable=SC2064
    trap "$cleanup" EXIT INT TERM
    export AWS_ACCESS_KEY_ID=fake_key
    export AWS_SECRET_ACCESS_KEY=fake_secret
    export AWS_REGION=us-west-2
    sleep 10
  fi

  if has_resource "dynamo"; then
    info_sub "creating dynamo container"
    dynID=$(docker run -itd --rm -p 4569:4569 -e SERVICES=dynamodb -e DATA_DIR=/tmp/localstack/data "localstack/localstack:$(get_resource_version "dynamo")")
    cleanup="$cleanup; docker stop $dynID"
    # shellcheck disable=SC2064
    trap "$cleanup" EXIT INT TERM
    sleep 10
  fi
fi

testInclude="$(get_repo_directory)/scripts/test.include.sh"
if [[ -e $testInclude ]]; then
  # Why: This is dynamic and can't be parsed
  # shellcheck disable=SC1090
  source "$testInclude"
fi

info "Running go test ($TEST_TAGS)"
# Why: We want these to split. For those wondering about "$@":
# https://stackoverflow.com/questions/5720194/how-do-i-pass-on-script-arguments-that-contain-quotes-spaces
# shellcheck disable=SC2086
"$DIR/gobin.sh" gotest.tools/gotestsum@v$(get_application_version "gotestsum") \
  --junitfile bin/unit-tests.xml --format dots-v2 -- $BENCH_FLAGS $COVER_FLAGS $TEST_FLAGS \
  -ldflags "-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing" -tags="$TEST_TAGS" \
  "$@" ./...

if [[ -n $CI ]]; then
  # Move this to a temporary directoy so that we can control
  # what gets uploaded via the store_test_results call
  mkdir -p /tmp/test-results
  mv bin/unit-tests.xml /tmp/test-results/
fi
