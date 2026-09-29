# linters

## Ignoring files

Files are ignored through the usage of `.gitignore`. This is done to ensure that the linters are not ran on files that are not meant to be committed.

There is currently no way to ignore files outside of the `.gitignore` file.

## Why linters is returning `return 1` after each command

There is currently a bash issue where the exit code is not getting detected when there is multiple commands being ran.
The easy solution for now is to return the value 1 from the function indicating an error has occurred and terminating the `make test/lint` command that triggered it

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
