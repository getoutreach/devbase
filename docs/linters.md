# linters

## Ignoring files

Files are ignored through the usage of `.gitignore`. This is done to ensure that the linters are not ran on files that are not meant to be committed.

There is currently no way to ignore files outside of the `.gitignore` file.

## Why linters is returning `return 1` after each command

There is currently a bash issue where the exit code is not getting detected when there is multiple commands being ran.
The easy solution for now is to return the value 1 from the function indicating an error has occurred and terminating the `make test/lint` command that triggered it

## Migrations Linter ##

Migrations linter runs for `.sql` files in a repository with a service.yaml that also includes
a postgreSQL block. Normal .sql, non-migration, sql files are ignored. 

The linter checks for required migrations file contents as well as contents that will 
break migrations. 

Disallowed keywords include: `CONCURRENTLY`, `ROLLBACK`, `BEGIN`, and `COMMIT`. The migrations 
linter also checks for proper `revises` directives in migration files.
