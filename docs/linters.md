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
