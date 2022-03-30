# application information
APP                 ?= unset
ORG                 ?= getoutreach
APP_VERSION         := $(shell git describe --match 'v[0-9]*' --tags --always HEAD 2>/dev/null || echo "0.0.0-dev")

# go options
GO                  ?= go
GOFMT               ?= gofmt
SHELL               := /usr/bin/env bash
GOOS                ?= $(shell go env GOOS)
GOARCH              ?= $(shell go env GOARCH)
PKG                 := $(GO) mod download -x
HONEYCOMB_API_KEY 	?= $(shell if [[ -f /run/secrets/outreach.io/honeycomb/apiKey ]]; then cat /run/secrets/outreach.io/honeycomb/apiKey; else cat ~/.outreach/$(APP)/honeycomb/apiKey; fi )
LDFLAGS             ?= $(shell if [[ -z $$DLV_PORT ]]; then printf -- "-w -s"; fi) -X github.com/getoutreach/gobox/pkg/app.Version=$(APP_VERSION) -X github.com/getoutreach/go-outreach/v2/pkg/app.Version=$(APP_VERSION) -X main.HoneycombTracingKey=$(HONEYCOMB_API_KEY)
GOFLAGS             :=
GOPRIVATE           := github.com/$(ORG)/*
GOPROXY             := https://proxy.golang.org
GO_EXTRA_FLAGS      := -v $(shell if [[ -z $$KUBERNETES_SERVICE_HOST ]]; then printf -- "-tags=or_dev"; fi)
TAGS                :=
BINDIR              := $(CURDIR)/bin
BIN_NAME            := $(APP)
PKGDIR              := github.com/$(ORG)/$(APP)
CGO_ENABLED         ?= 1
TOOL_DEPS           := ${GO}

# formatters / misc
CLANG_FORMAT        ?= ./scripts/shell-wrapper.sh clang-format.sh
JSONNETFMT          ?= ./scripts/shell-wrapper.sh gobin.sh github.com/google/go-jsonnet/cmd/jsonnetfmt@v0.16.0
LOG                 := ./scripts/shell-wrapper.sh makefile-logger.sh
FLY                 ?= $(shell ./scripts/shell-wrapper.sh gobin.sh -p github.com/concourse/concourse/fly@cfe7746ae74247743708be6c5b2f40215030a1f1)

# Testing options
BENCH_FLAGS         := "-bench=Bench $(BENCH_FLAGS)"
TEST_TAGS           ?= or_test,or_int
SKIP_VALIDATE       ?=

# Outreach stuff, will be moved out... one day.
OUTREACH_DOMAIN     ?= outreach-dev.com
ACCOUNTS_URL        ?= https://accounts.$(OUTREACH_DOMAIN)

# E2E
BASE_TEST_ENV       ?= GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) SKIP_VALIDATE=${SKIP_VALIDATE} OSS=$(OSS)
E2E_NAMESPACE       ?= $(APP)--bento1a
E2E_SERVICE_ACCOUNT ?= $(APP)-e2e-client-svc
E2E_CLUSTER         ?= development.us-west-2
E2E_ENVIRONMENT     ?= development
OSS                 ?= false

.PHONY: default
default: build

# All the pre-action steps are bunched together here.
.PHONY: pre-release pre-build pre-test pre-coverage pre-integration pre-e2e pre-benchmark pre-gogenerate pre-devserver pre-debug pre-docker-build pre-fmt pre-docs pre-docs-publish
pre-release::
pre-build::
pre-test::
pre-coverage::
pre-integration::
pre-e2e::
pre-benchmark::
pre-gogenerate::
pre-devserver::
pre-dev::
pre-debug::
pre-docker-build::
pre-fmt::
pre-docs::
pre-docs-publish::

## release:         tag a new release with goreleaser
.PHONY: release
release:: pre-release
	@# Create a tag for our version
	@git tag -d "$(APP_VERSION)" >&2 || true
	@git tag "$(APP_VERSION)" >&2
	@GORELEASER_CURRENT_TAG=$(APP_VERSION) ./scripts/shell-wrapper.sh gobin.sh github.com/goreleaser/goreleaser@v1.4.1 release --skip-announce --skip-publish --skip-validate --rm-dist
	@# Delete the tag once we are done.
	@git tag -d "$(APP_VERSION)" >&2

## help             show this help
.PHONY : help
help: Makefile
	@printf "\n[running make with no target runs make build]\n\n"
	@sed -n 's/^##[^#]//p' .bootstrap/root/Makefile Makefile

## check-deps:      check for required dependencies
.PHONY: check-deps
check-deps:
	@[[ ! -z "${SKIP_CHECK_DEPS}" ]] || ./scripts/shell-wrapper.sh check_deps.sh ${TOOL_DEPS}

## pre-commit:      run housekeeping utilities before creating a commit
.PHONY: pre-commit
pre-commit: fmt

## build:           run codegen and build application binary
.PHONY: build
build:: pre-build gobuild

## test:            run unit tests
.PHONY: test
test:: pre-test
	$(BASE_TEST_ENV) ./scripts/shell-wrapper.sh test.sh

## coverage:        generate code coverage
.PHONY: coverage
coverage:: pre-coverage
	 WITH_COVERAGE=true GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) ./scripts/shell-wrapper.sh test.sh
	 go tool cover --html=/tmp/coverage.out

## integration:     run integration tests
.PHONY: integration
integration:: pre-integration
	TEST_TAGS=${TEST_TAGS} $(BASE_TEST_ENV) ./scripts/shell-wrapper.sh test.sh

.PHONY: e2e
e2e:: pre-e2e
	E2E=true OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) MY_CLUSTER=$(E2E_CLUSTER) MY_ENVIRONMENT=$(E2E_ENVIRONMENT) MY_POD_SERVICE_ACCOUNT=$(E2E_SERVICE_ACCOUNT) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) ./scripts/shell-wrapper.sh ci/testing/setup-devenv.sh

## benchmark:       run benchmarks
.PHONY: benchmark
benchmark:: pre-benchmark
	BENCH_FLAGS=${BENCH_FLAGS} TEST_TAGS=${TEST_TAGS} $(BASE_TEST_ENV) SKIP_VALIDATE=true ./scripts/shell-wrapper.sh test.sh | tee /tmp/benchmark.txt
	@$(LOG) info "Results of benchmarks: "
	./scripts/shell-wrapper.sh gobin.sh golang.org/x/perf/cmd/benchstat@03971e38 /tmp/benchmark.txt

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
	./scripts/shell-wrapper.sh grpcui.sh localhost:5000
## dev:       run the service
.PHONY: dev
dev:: pre-dev
	@./scripts/shell-wrapper.sh gobin.sh github.com/cosmtrek/air@v1.27.8 -c $(CURDIR)/.air.toml

## devserver:       run the service
.PHONY: devserver
devserver:: pre-devserver build
	if [[ -z $$SKIP_DEVCONFIG ]]; then ./scripts/shell-wrapper.sh devconfig.sh; fi
	OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) MY_CLUSTER=$(E2E_CLUSTER) MY_ENVIRONMENT=$(E2E_ENVIRONMENT) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) $(BINDIR)/$(BIN_NAME)

## devserver:       run the service with live reloading
.PHONY: devserver-reload
devserver-reload:: pre-devserver build
	if [[ -z $$SKIP_DEVCONFIG ]]; then ./scripts/shell-wrapper.sh devconfig.sh; fi
	OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) MY_CLUSTER=$(E2E_CLUSTER) MY_ENVIRONMENT=$(E2E_ENVIRONMENT) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) ./scripts/shell-wrapper.sh live-reload.sh

## debug:           run the service via delve
.PHONY: debug
debug:: pre-debug build
	if [[ -z $$SKIP_DEVCONFIG ]]; then ./scripts/shell-wrapper.sh devconfig.sh; fi
	if [[ -n $$DLV_PORT ]]; then ./scripts/shell-wrapper.sh debug.sh; fi
	if [[ -z $$DLV_PORT ]]; then OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) ./scripts/shell-wrapper.sh debug.sh; fi

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
	@./scripts/shell-wrapper.sh fmt.sh

## docs:            build protobuf/TypeScript HTML docs
.PHONY: docs
docs:: pre-docs
	@./.bootstrap/shell/docs-build.sh

## docs-publish:    publish protobuf/TypeScript HTML docs to GitHub Pages
.PHONY: docs-publish
docs-publish:: pre-docs-publish
	@./.bootstrap/shell/docs-publish.sh

## fly-login:       log into concourse
.PHONY: fly-login
fly-login:
	${FLY} -t devs userinfo || ${FLY} -t devs login --concourse-url https://concourse.outreach.cloud -n devs

## update-pipeline: update application pipeline in concourse
.PHONY: update-pipeline
update-pipeline: fly-login
	@echo " ===> updating concourse pipeline <==="
	@if [[ ! -e "concourse/jsonnet-libs" ]]; then git clone https://github.com/getoutreach/jsonnet-libs concourse/jsonnet-libs; else cd concourse/jsonnet-libs && git reset --hard origin/master && git pull; fi
	cd concourse; rm -f /tmp/pipeline.yml; jsonnet -J ./jsonnet-libs -y pipeline.jsonnet > /tmp/pipeline.yml && ${FLY} -t devs sp -c /tmp/pipeline.yml -p $(APP)

.PHONY: version
version:
	@echo "$(APP_VERSION)"

## benchmarker-gen:	generate a benchmarker input file using your locally running gRPC server
.PHONY: benchmarker-gen
benchmarker-gen:
	benchmarker grpc --server localhost:5000 --output benchmarker-grpc --format json --ignore-service Throttler

## benchmarker-run:	run the generated benchmarker input file using your locally running gRPC server
.PHONY: benchmarker-run
benchmarker-run:
	benchmarker --input benchmarker-grpc.json --output benchmarker-output.json --format json --verbose
