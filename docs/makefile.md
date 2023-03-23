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
* `RACE` (env var): Enables `-race` testflag (default: ''). Set to `disabled` to disable.
* `SHUFFLE` (env var): Enables `-shuffle` testflag (default: ''). Set to `disabled` to disable.
* `TEST_PACKAGES` (env var): Packages to test. Defaults to `./...`
* `PACKAGE_TO_DEBUG` (env var): Set to debug a specific package.

### `lint`

Runs the linters for the project. This defaults to running all linters.

See [linters](linters.md) for more information.

### `coverage`

**Note**: The options here are shared with `make test`.

Runs the tests for the project and generates a coverage report. This defaults to running all linters and validators. Then finally `go test` is ran with coverage enabled.

### `fmt`

Runs all formatting tools for the project.

Formatters share most of the same options as `make lint`, see the [linters](linters.md) documentation for more information.

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

Runs a Go application in debug mode using [delve](https://github.com/go-delve/delve).

#### Environment Variables

* `PACKAGE_TO_DEBUG`: Path to package to debug. Defaults to `./cmd/$(APP_NAME)`.
* `IN_CONTAINER`: Overrides the `IN_CONTAINER` environment variable. This is set automatically when running in a container.
* `DLV_PORT`: Port to run the debugger on, must be set when `HEADLESS` is true.
* `HEADLESS`: Run in headless mode. Defaults to `false` when `IN_CONTAINER` is `false`. Must set `DLV_PORT` when `HEADLESS` is `true`.
* `DEV_CONTAINER_LOGFILE`: Path to log file to use when running in a container. Defaults to `/tmp/app.log`.

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

### `e2e`

Runs tests marked with `or_e2e` build tags after provisioning a [devenv](github.com/getoutreach/devenv).
If you have a file in your repository, `scripts/devenv/post-e2e-deploy.sh`, it
will run it right after the devenv has been provisioned (before the tests run).

#### Environment Variables

* `SKIP_DEVENV_PROVISION`: Set "true" to skip provision step. Default false
* `PROVISION_TARGET`: Maps to `devenv provision --snapshot-target $PROVISION_TARGET`, allowing to specify the provision target used. Otherwise, the default is either "flagship" or "base", latter being used when "outreach" is not included.
* `SKIP_LOCALIZER`: Set "true" to skip creating a localizer tunnel before test start.
* `REQUIRE_DEVCONFIG_AFTER_DEPLOY`: Set to "true" to run `devconfig.sh` after deploy. Otherwise, the step is executed before deploy.
