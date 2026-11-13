# linters

## Ignoring files

Files are ignored through the usage of `.gitignore`. This is done to ensure that the linters are not ran on files that are not meant to be committed.

There is currently no way to ignore files outside of the `.gitignore` file.

## Why linters is returning `return 1` after each command

There is currently a bash issue where the exit code is not getting detected when there is multiple commands being ran.
The easy solution for now is to return the value 1 from the function indicating an error has occurred and terminating the `make test/lint` command that triggered it

## Go directive check

The Go linter fails if the `go` directive of a `go.mod` that Stencil manages (one
listed in `stencil.lock`) is not the Go version that Stencil provides. That is the
`go.stanza` argument in `service.yaml` if set, otherwise the `golang` version in
`.tool-versions`, which Stencil writes from `versions.go`. This catches changes
that Stencil did not make, such as a dependency update. To use another Go
version, change `versions.go` in `service.yaml` or update `stencil-golang`, and
run `stencil`.

The Go linter also fails if the `toolverOverride` block of `.tool-versions` sets
`golang`, because the Go version in use would then differ from the one in
`go.mod`. Set `versions.go` in `service.yaml` instead. This applies only to
repositories where Stencil manages the `go.mod` file in the repository root.

If `go.stanza` is set, the linter compares the `go` directive with it, and not
with `.tool-versions`.

## CODEOWNERS linter

The `codeowners` linter runs `devbase lint codeowners` when the repository has a CODEOWNERS file in the root, `docs/`, or `.github/` directory. It validates the owners on GitHub only when CODEOWNERS changed since the merge base with the default branch. It has no formatter. See [CI](codeowners.md#ci) in the CODEOWNERS linter documentation.

A CODEOWNERS file has no extension, so the linter sets `extensions` to an empty list. An empty list matches every file, so the linter always runs, and it skips a repository without a CODEOWNERS file.

## Project specific linters

Projects can create additional linters to be run in addition to the built-in
linters.

To add a linter place the linter shell script in `scripts/linters/<lintername>.sh`.
The linter will be discovered when globbing `.sh` files run with the built-in
linters. Follow the conventions of the existing linter shell scripts when creating
the new linter.

## Running formatters via mise

`mise run fmt` is the mise-native equivalent of `make fmt`. `mise run fmt:<name>`
(e.g. `mise run fmt:go`, `mise run fmt:bash`) runs a single formatter. Run
`mise tasks ls` to see the full list of `fmt:*` tasks.

Unlike `make fmt`, which auto-discovers `scripts/linters/*.sh` by globbing,
`mise run fmt` only runs formatters registered as `fmt:<name>` mise tasks. A
project-specific linter (added as above) needs its own `fmt:<name>` task to be
picked up by `mise run fmt`, via either:

1. A file-based task at `.mise/tasks/fmt/<name>` that delegates to devbase's
   shared runner via `scripts/shell-wrapper.sh` (the same wrapper `make fmt`
   itself uses to reach devbase's vendored shell scripts):

   ```bash
   #!/usr/bin/env bash
   #MISE description="Formats <your language> files."

   set -euo pipefail

   exec "$MISE_PROJECT_ROOT/scripts/shell-wrapper.sh" run-formatter.sh "$MISE_PROJECT_ROOT/scripts/linters/<name>.sh"
   ```

2. A plain TOML task in your own `mise.toml`, with no dependency on the
   `extensions`/`formatter()` convention at all:

   ```toml
   [tasks."fmt:<name>"]
   description = "Formats <your language> files."
   run = "your-formatter-command --write ."
   ```

Both forms are discovered identically by `mise run fmt`, since mise treats
file-based and TOML-defined tasks the same way for task lookup.
