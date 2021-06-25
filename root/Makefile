# go option
GO                  ?= go
GOFMT               ?= gofmt
CLANG_FORMAT        ?= ./.bootstrap/shell/clang-format.sh
JSONNETFMT          ?= ./.bootstrap/shell/gobin.sh github.com/google/go-jsonnet/cmd/jsonnetfmt@v0.16.0
SHELL               := /usr/bin/env bash
GOOS                ?= $(shell go env GOOS)
GOARCH              ?= $(shell go env GOARCH)
PKG                 := $(GO) mod download -x
APP_VERSION         := $(shell git describe --match 'v[0-9]*' --tags --always HEAD)
LDFLAGS             := -w -s -X github.com/getoutreach/gobox/pkg/app.Version=$(APP_VERSION) -X github.com/getoutreach/go-outreach/v2/pkg/app.Version=$(APP_VERSION) -X main.HoneycombTracingKey=$(shell cat ~/.outreach/$(APP)/honeycomb/apiKey)
GOFLAGS             :=
LOG                 := "$(CURDIR)/.bootstrap/shell/makefile-logger.sh"
GOPRIVATE           := github.com/getoutreach/*
GOPROXY             := https://proxy.golang.org
GO_EXTRA_FLAGS      := -v -tags=or_dev
TAGS                :=
BINDIR              := $(CURDIR)/bin
BIN_NAME            := $(APP)
PKGDIR              := github.com/getoutreach/$(APP)
CGO_ENABLED         ?= 1
TOOL_DEPS           := ${GO}
BENCH_FLAGS         := "-bench=Bench $(BENCH_FLAGS)"
TEST_TAGS           ?= or_test,or_int
SKIP_VALIDATE       ?=
FLY                 ?= $(shell ./.bootstrap/shell/gobin.sh -p github.com/concourse/concourse/fly@cfe7746ae74247743708be6c5b2f40215030a1f1)
E2E_ARGS            ?=  --deploy-app giraffe --deploy-app authz
E2E_NAMESPACE       ?= $(APP)--bento1a
E2E_SERVICE_ACCOUNT ?= $(APP)-e2e-client-svc
OUTREACH_DOMAIN     ?= outreach-dev.com
ACCOUNTS_URL        ?= https://accounts.$(OUTREACH_DOMAIN)
BASE_TEST_ENV       ?= GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) SKIP_VALIDATE=${SKIP_VALIDATE}


.PHONY: default
default: build

# All the pre-action steps are bunched together here.
.PHONY: pre-release pre-build pre-test pre-coverage pre-integration pre-e2e pre-benchmark pre-gogenerate pre-devserver pre-debug pre-docker-build pre-fmt
pre-release::
pre-build::
pre-test::
pre-coverage::
pre-integration::
pre-e2e::
pre-benchmark::
pre-gogenerate::
pre-devserver::
pre-debug::
pre-docker-build::
pre-fmt::


## release:         tag a new release with goreleaser
.PHONY: release
release:: pre-release
	@# Create a tag for our version
	@git tag -d "$(APP_VERSION)" >&2 || true
	@git tag "$(APP_VERSION)" >&2
	@./.bootstrap/shell/gobin.sh github.com/goreleaser/goreleaser@v0.157.0 release --skip-publish --rm-dist
	@# Delete the tag once we\'re done.
	@git tag -d "$(APP_VERSION)" >&2

## help             show this help
.PHONY : help
help: Makefile
	@printf "\n[running make with no target runs make build]\n\n"
	@sed -n 's/^##[^#]//p' .bootstrap/root/Makefile Makefile

## check-deps:      check for required dependencies
.PHONY: check-deps
check-deps:
	@[[ ! -z "${SKIP_CHECK_DEPS}" ]] || ./.bootstrap/shell/check_deps.sh ${TOOL_DEPS}

## pre-commit:      run housekeeping utilities before creating a commit
.PHONY: pre-commit
pre-commit: fmt

## build:           run codegen and build application binary
.PHONY: build
build:: pre-build gobuild

## test:            run unit tests
.PHONY: test
test:: pre-test
	$(BASE_TEST_ENV) ./.bootstrap/shell/test.sh

## coverage:        generate code coverage
.PHONY: coverage
coverage:: pre-coverage
	 WITH_COVERAGE=true GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) ./.bootstrap/shell/test.sh
	 go tool cover --html=/tmp/coverage.out

## integration:     run integration tests
.PHONY: integration
integration:: pre-integration
	TEST_TAGS=${TEST_TAGS} $(BASE_TEST_ENV) ./.bootstrap/shell/test.sh

## e2e:             run e2e tests
.PHONY: e2e
e2e:: pre-e2e
	@devenv --skip-update status -q || \
		(echo "Starting developer environment"; set -x; devenv --skip-update provision ${E2E_ARGS})
	TEST_TAGS=or_test,or_e2e $(BASE_TEST_ENV) MY_NAMESPACE=$(E2E_NAMESPACE) MY_POD_SERVICE_ACCOUNT=$(E2E_SERVICE_ACCOUNT) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) ./.bootstrap/shell/test.sh

## benchmark:       run benchmarks
.PHONY: benchmark
benchmark:: pre-benchmark
	BENCH_FLAGS=${BENCH_FLAGS} TEST_TAGS=${TEST_TAGS} $(BASE_TEST_ENV) ./.bootstrap/shell/test.sh | tee /tmp/benchmark.txt
	@$(LOG) info "Results of benchmarks: "
	./.bootstrap/shell/gobin.sh golang.org/x/perf/cmd/benchstat /tmp/benchmark.txt

## dep:             download go dependencies
.PHONY: dep
dep:
	@$(LOG) info "Installing dependencies via '$(PKG)'"
	GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) $(PKG)

## gogenerate:      run go codegen
.PHONY: gogenerate
gogenerate:: pre-gogenerate check-deps
	@$(LOG) info "Running gogenerate"
	@GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) $(GO) generate ./...

## gobuild:         build application binary
.PHONY: gobuild
gobuild: check-deps
	@$(LOG) info "Building binaries into ./bin/"
	mkdir -p $(BINDIR)
	GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o $(BINDIR)/ -ldflags "$(LDFLAGS)" $(GO_EXTRA_FLAGS) $(PKGDIR)/...
## grpcui:          run grpcui for an already locally running service
.PHONY: grpcui
grpcui:
	@$(LOG) info "Launching gRPCUI"
	./.bootstrap/shell/grpcui.sh localhost:5000
## devserver:       run the service
.PHONY: devserver
devserver:: pre-devserver build
	if [[ -z $$SKIP_DEVCONFIG ]]; then ./.bootstrap/shell/devconfig.sh; fi
	OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) $(BINDIR)/$(BIN_NAME)

## debug:           run the service via delve
.PHONY: debug
debug:: pre-debug build
	if [[ -z $$SKIP_DEVCONFIG ]]; then ./.bootstrap/shell/devconfig.sh; fi
	OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) ./.bootstrap/shell/debug.sh

## docker-build:    build docker image for dev environment
.PHONY: docker-build
docker-build:: pre-docker-build
	@echo " ===> building docker image <==="
	@ssh-add -L
	@echo " ===> If you run into credential issues, ensure that your key is in your SSH agent (ssh-add <ssh-key-path>) <==="
	DOCKER_BUILDKIT=1 docker build --ssh default -t gcr.io/outreach-docker/$(APP) -f deployments/$(APP)/Dockerfile . --build-arg VERSION=${APP_VERSION}

## fmt:             run source code formatters
.PHONY: fmt
fmt:: pre-fmt
	@./.bootstrap/shell/fmt.sh

## fly-login:       log into concourse
.PHONY: fly-login
fly-login::
	${FLY} -t devs userinfo || ${FLY} -t devs login --concourse-url https://concourse.outreach.cloud -n devs

## update-pipeline: update application pipeline in concourse
.PHONY: update-pipeline
update-pipeline:: fly-login
	@echo " ===> updating concourse pipeline <==="
	@if [[ ! -e "concourse/jsonnet-libs" ]]; then git clone https://github.com/getoutreach/jsonnet-libs concourse/jsonnet-libs; else cd concourse/jsonnet-libs && git reset --hard origin/master && git pull; fi
	cd concourse; rm -f /tmp/pipeline.yml; jsonnet -J ./jsonnet-libs -y pipeline.jsonnet > /tmp/pipeline.yml && ${FLY} -t devs sp -c /tmp/pipeline.yml -p $(APP)

.PHONY: version
version:
	@echo "$(APP_VERSION)"
