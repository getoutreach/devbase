# Makefile

Our current `make` infrastructure is in the process of being replaced with a tool called "mage". The new tool is much more flexible and easier to use. The new tool is also written in Go, which means it's much easier to maintain and extend.

For now you can call all targets using `make` as usual, as per the docs below.

## Makefile Targets

### `help`

Shows a list of all targets and short descriptions.

### `build'

Builds the project. This is the default target. This defaults to building a go application.

### `test`

Runs the tests for the project. This defaults to running all linters and validators. Then finally `go test` is ran.

This uses [gotestsum](https://github.com/gotestyourself/gotestsum) to run the tests for better output as well as automatic junit output in CI.

#### Options

* **Deprecated** (use `SKIP_LINTERS` instead) `SKIP_VALIDATE` (env var): Skips running the validators (linters)
* `SKIP_LINTERS` (env var): Skips running the linters
* `COVER_FLAGS` (env var): Flags to pass to `go test -cover` when coverage is enabled
* `TEST_FLAGS` (env var): Flags to pass to `go test`
* `BENCH_FLAGS` (env var): Flags to pass to `go test -bench` when `make benchmark` is run
* `GO_TEST_TIMEOUT` (env var): Timeout for `go test`
* `TEST_TAGS` (env var): Tags to pass to `go test` (default: `or_test`)
* `RACE` (env var): Enables race condition testings (default: ''). Set to `disabled` to disable.
* `TEST_PACKAGES` (env var): Packages to test. Defaults to `./...`
* `PACKAGE_TO_DEBUG` (env var): Set to debug a specific package.

### `lint`

Runs the linters for the project. This defaults to running all linters.

### `coverage`

**Note**: The options here are shared with `make test`.

Runs the tests for the project and generates a coverage report. This defaults to running all linters and validators. Then finally `go test` is ran with coverage enabled.

### `fmt`

Runs all formatting tools for the project.

### `gogenerate`

Runs `go generate` on the project.

### `grpcui`

Runs [grpcui](https://github.com/fullstorydev/grpcui)

### `benchmark`

Runs the benchmarks for the project. This defaults to running all linters and validators. Then finally `go test -bench` is ran.

### `run`

Runs the project. This defaults to running a go application.

### `dev`

Runs `make run` but first runs `make devconfig` to ensure all configuration is up to date.

### `devconfig`

Runs `make gogenerate` to ensure all configuration is up to date.

### `devserver`

**Deprecated**: Use `make dev` instead.

Alias for `make dev`.

### `debug`

Runs the project in debug mode. This defaults to running a go application.

### `docs`

Builds the documentation for the project.

### `update-pipeline`

Updates the Concourse pipeline for the project.

### `deploy`

Deploys the project to Maestro, an internal Outreach service.

### `version`

Returns the current application version

### `gobuild`

Runs `go build` on the project with a set of linker variables.

### `dep`

Installs all Go dependencies
