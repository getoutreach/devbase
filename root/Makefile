SHELL                := /usr/bin/env bash

YQ                   := $(CURDIR)/.bootstrap/shell/yq.sh
BOOTSTRAP_DIR        := $(shell if [[ "$$("$(YQ)" -r .name service.yaml 2>/dev/null || echo "")" == "devbase" ]]; then echo "$(CURDIR)" ; else echo "$(CURDIR)/.bootstrap"; fi)

# This ensures that all of the devbase tool-versions have been installed before
# running any of the targets. This is especially important for the mage,
# which we execute as part of the Makefile.
#
# We do 1>&2 here to ensure that output isn't swallowed by the
# ENSURE_TOOL_VERSIONS variable. Otherwise, asdf tools are silently installed.
ENSURE_TOOL_VERSIONS := $(shell "$(BOOTSTRAP_DIR)/root/ensure_asdf.sh" 1>&2)

# application information
APP                 ?= unset
ORG                 ?= getoutreach

# Transition
MAGE_CMD            := ASDF_MAGE_VERSION=$(shell cat "$(BOOTSTRAP_DIR)/.tool-versions" | grep mage | awk '{ print $$2 }') MAGEFILE_HASHFAST=true mage -d "$(CURDIR)/$(shell if [[ "$(APP)" != "devbase" ]]; then echo ".bootstrap/"; fi)root" -w "$(CURDIR)"
APP_VERSION         := $(shell $(MAGE_CMD) version)

# go options
GO                  ?= go
GOOS                ?= $(shell go env GOOS)
GOARCH              ?= $(shell go env GOARCH)
GOFLAGS             :=
GOPRIVATE           := github.com/$(ORG)/*
TAGS                :=
BINDIR              := $(CURDIR)/bin
BIN_NAME            := $(APP)
PKGDIR              := github.com/$(ORG)/$(APP)
CGO_ENABLED         ?= $(shell "$(CURDIR)/scripts/shell-wrapper.sh" cgo-enabled.sh)
TOOL_DEPS           := ${GO}

# formatters / misc
JSONNET             ?= $(CURDIR)/scripts/shell-wrapper.sh gobin.sh github.com/google/go-jsonnet/cmd/jsonnet@v0.19.0
LOG                 := ./scripts/shell-wrapper.sh makefile-logger.sh
BOX_CONFIG          := ./scripts/shell-wrapper.sh boxconfig.sh
AIR                 := $(CURDIR)/scripts/shell-wrapper.sh gobin.sh github.com/cosmtrek/air@v1.44.0 -c $(CURDIR)/.air.toml

# Testing options
BENCH_FLAGS         := "-bench=Bench $(BENCH_FLAGS)"
TEST_TAGS           ?= or_test
SKIP_VALIDATE       ?=
LOGFMT              ?=
GOLANGCI_LINT_CACHE ?= "$(HOME)/.outreach/.cache/.golangci-lint/$(APP)"

# Outreach stuff, will be moved out... one day.
OUTREACH_DOMAIN     ?= outreach-dev.com
ACCOUNTS_URL        ?= https://accounts.$(OUTREACH_DOMAIN)

# E2E
BASE_TEST_ENV       ?= GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) SKIP_VALIDATE=${SKIP_VALIDATE} SKIP_LINTERS=${SKIP_LINTERS} OSS=$(OSS) TEST_TAGS=$(TEST_TAGS) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE)
E2E_NAMESPACE       ?= $(APP)--bento1a
E2E_SERVICE_ACCOUNT ?= $(APP)-e2e-client-svc
E2E_CLUSTER         ?= development.us-west-2
E2E_ENVIRONMENT     ?= development
OSS                 ?= false

# Docker build
BASE_IMAGE          ?= gcr.io/outreach-docker/$(APP)
DOCKERFILE          ?= deployments/$(APP)/Dockerfile

.PHONY: default
default: build

# All the pre-action steps are bunched together here.
.PHONY: pre-release pre-build pre-lint pre-run pre-test pre-coverage pre-integration pre-e2e pre-benchmark pre-gogenerate pre-devconfig pre-devserver pre-dev pre-debug pre-docker-build pre-fmt pre-docs pre-docs-publish pre-stencil post-stencil
pre-release::
pre-build::
pre-run::
pre-lint::
pre-test::
pre-coverage::
pre-integration::
pre-e2e::
pre-benchmark::
pre-gogenerate::
pre-devconfig::
pre-devserver::
pre-dev::
pre-debug::
pre-docker-build::
pre-fmt::
pre-docs::
pre-docs-publish::
pre-stencil::
post-stencil::

## release:         tag a new release with goreleaser
.PHONY: release
release:: pre-release
	@# Create a tag for our version
	@git tag -d "$(APP_VERSION)" >&2 >/dev/null || true
	@git tag "$(APP_VERSION)" >&2
	@GORELEASER_CURRENT_TAG=$(APP_VERSION) ASDF_GORELEASER_VERSION=$(shell cat "$(BOOTSTRAP_DIR)/.tool-versions" | grep goreleaser | awk '{ print $$2 }') \
		goreleaser release --skip-announce --skip-publish --skip-validate --clean
	@# Delete the tag once we are done.
	@git tag -d "$(APP_VERSION)" >&2

## help             show this help
.PHONY: help
help: Makefile
	@printf "\n[running make with no target runs make build]\n\n"
	@sed -n 's/^##[^#]//p' .bootstrap/root/Makefile Makefile
	@printf "\n"
	@echo "Mage (make <target>) targets"
	@$(MAGE_CMD) -l

## pre-commit:      run housekeeping utilities before creating a commit
.PHONY: pre-commit
pre-commit: fmt

## build:           run codegen and build application binary
.PHONY: build
build:: pre-build gobuild

## lint:            run code linters
.PHONY: lint
# Note: We run pre-test for compat. Remove on the next breaking change.
lint:: pre-test pre-lint
	@# Note that this requires the ensure_asdf.sh invocation at the top of
	@# this file.
	$(BASE_TEST_ENV) ./scripts/shell-wrapper.sh linters.sh

## test:            run unit tests
.PHONY: test
test:: pre-test lint
	$(BASE_TEST_ENV) ./scripts/shell-wrapper.sh test.sh

## test-e2e:        run only e2e test (use inside a dev pod)
.PHONY: test-e2e
test-e2e:: pre-test
	$(BASE_TEST_ENV) TEST_TAGS=or_test,or_e2e ./scripts/shell-wrapper.sh test.sh

## coverage:        generate code coverage
.PHONY: coverage
coverage:: pre-coverage
	 WITH_COVERAGE=true GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) ./scripts/shell-wrapper.sh test.sh
	 go tool cover --html=/tmp/coverage.out

.PHONY: e2e
e2e:: pre-e2e
	$(BASE_TEST_ENV) E2E=true OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) \
	MY_CLUSTER=$(E2E_CLUSTER) MY_ENVIRONMENT=$(E2E_ENVIRONMENT) \
	MY_POD_SERVICE_ACCOUNT=$(E2E_SERVICE_ACCOUNT) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) \
	./scripts/shell-wrapper.sh ci/testing/setup-devenv.sh

## benchmark:       run benchmarks
.PHONY: benchmark
benchmark:: pre-benchmark
	BENCH_FLAGS=${BENCH_FLAGS} TEST_TAGS=${TEST_TAGS} $(BASE_TEST_ENV) SKIP_VALIDATE=true ./scripts/shell-wrapper.sh test.sh | tee /tmp/benchmark.txt
	@$(LOG) info "Results of benchmarks: "
	./scripts/shell-wrapper.sh gobin.sh golang.org/x/perf/cmd/benchstat@03971e38 /tmp/benchmark.txt

## gogenerate:      run go codegen
.PHONY: gogenerate
gogenerate:: pre-gogenerate
	@$(LOG) info "Running gogenerate"
	@GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) $(GO) generate ./...

## gobuild:         build application binary
.PHONY: gobuild
gobuild:
	@CGO_ENABLED=$(CGO_ENABLED) $(MAGE_CMD) gobuild

## grpcui:          run grpcui for an already locally running service
.PHONY: grpcui
grpcui:
	@$(LOG) info "Launching gRPCUI"
	./scripts/shell-wrapper.sh grpcui.sh localhost:5000

## run:           run the service - the right binary, with opt-in logfmt
.PHONY: run
run:: pre-run build
	@DEVBOX_LOGFMT="$(LOGFMT)" ./scripts/shell-wrapper.sh air-runner.sh

## devspace-watch:         watch source files and build them for linux. To be used with devspace using `devenv apps run --sync-binaries .`
.PHONY: devspace-watch
devspace-watch:
	@DEVBOX_LOGFMT="$(LOGFMT)" BUILD_FOR_GOOS="linux" CGO_ENABLED=$(CGO_ENABLED) SKIP_TRIMPATH="true" SKIP_STARTING_APP="true" DLV_PORT=42097 $(AIR)

## devspace:               build sources for linux with debugging symbols. To be used with devspace using `devenv apps run --sync-binaries .`
.PHONY: devspace
devspace:
	@DEVBOX_LOGFMT="$(LOGFMT)" BUILD_FOR_GOOS="linux" CGO_ENABLED=$(CGO_ENABLED) SKIP_TRIMPATH="true" SKIP_STARTING_APP="true" DLV_PORT=42097 GC_FLAGS="all=-N -l" $(MAGE_CMD) gobuild
	@DEVBOX_LOGFMT="$(LOGFMT)" BUILD_FOR_GOOS="linux" CGO_ENABLED=$(CGO_ENABLED) $(MAGE_CMD) e2etestbuild
	@echo "Use 'devenv apps run --sync-binaries .' to sync binaries to devspace pod"

## dev:       run the service
.PHONY: dev
dev:: pre-dev
	@DEVBOX_LOGFMT="$(LOGFMT)" $(AIR)

## devconfig:       generates the required configuration to run the service locally
.PHONY: devconfig
devconfig:: pre-devconfig
	if [[ -z $$SKIP_DEVCONFIG ]]; then ./scripts/shell-wrapper.sh devconfig.sh; fi

## devserver:       run the service
.PHONY: devserver
devserver:: pre-devserver build devconfig
	OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) MY_CLUSTER=$(E2E_CLUSTER) MY_ENVIRONMENT=$(E2E_ENVIRONMENT) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) MY_POD_SERVICE_ACCOUNT=$(E2E_SERVICE_ACCOUNT) $(BINDIR)/$(BIN_NAME)

## debug:           run the service via delve
.PHONY: debug
debug:: pre-debug
	@if [[ -z $$SKIP_DEVCONFIG ]]; then DEVBOX_LOGFMT="$(LOGFMT)" ./scripts/shell-wrapper.sh devconfig.sh; fi
	@if [[ -n $$DLV_PORT ]]; then DEVBOX_LOGFMT="$(LOGFMT)" ./scripts/shell-wrapper.sh debug.sh; fi
	@if [[ -z $$DLV_PORT ]]; then OUTREACH_ACCOUNTS_BASE_URL=$(ACCOUNTS_URL) MY_NAMESPACE=$(E2E_NAMESPACE) OUTREACH_DOMAIN=$(OUTREACH_DOMAIN) ./scripts/shell-wrapper.sh debug.sh; fi

## docker-build:    build docker image for dev environment
.PHONY: docker-build
docker-build:: pre-docker-build
	@echo " ===> building docker image <==="
	@ssh-add -L
	@echo " ===> If you run into credential issues, ensure that your key is in your SSH agent (ssh-add <ssh-key-path>) <==="
	DOCKER_BUILDKIT=1 docker build --ssh default -t $(BASE_IMAGE) -f $(DOCKERFILE) . --build-arg VERSION=${APP_VERSION} $(DOCKER_BUILD_EXTRA_ARGS)

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

## stencil:         run stencil
.PHONY: stencil
stencil:: pre-stencil run-stencil post-stencil

.PHONY: run-stencil
run-stencil:
	stencil --skip-update

## restencil:       run stencil without updating module versions
.PHONY: restencil
restencil:: pre-stencil run-restencil post-stencil

.PHONY: run-restencil
run-restencil:
	stencil --skip-update --frozen-lockfile

## benchmarker-gen:	generate a benchmarker input file using your locally running gRPC server
.PHONY: benchmarker-gen
benchmarker-gen:
	benchmarker grpc --server localhost:5000 --output benchmarker-grpc --format json --ignore-service Throttler

## benchmarker-run:	run the generated benchmarker input file using your locally running gRPC server
.PHONY: benchmarker-run
benchmarker-run:
	benchmarker --input benchmarker-grpc.json --output benchmarker-output.json --format json --verbose

## deploy: update actionable version to maestro
.PHONY: deploy
deploy:
	@$(MAGE_CMD) deploy $(APP) $(CHANNEL)

# Catch all to mage
%::
	@$(MAGE_CMD) $@
