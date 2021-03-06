# devbase

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/getoutreach/devbase)
[![Generated via Bootstrap](https://img.shields.io/badge/Outreach-Bootstrap-%235951ff)](https://github.com/getoutreach/bootstrap)
[![Coverage Status](https://coveralls.io/repos/github/getoutreach/devbase/badge.svg?branch=main)](https://coveralls.io/github//getoutreach/devbase?branch=main)

A collection of scripts and ci configuration

## Contributing

Please read the [CONTRIBUTING.md](CONTRIBUTING.md) document for guidelines on developing and contributing changes.

## High-level Overview

<!--- Block(overview) -->

## How to use a Custom Build of `devbase`

### Bootstrap / Stencil

To test a custom build of `devbase` with bootstrap, simply modify `bootstrap.lock` to point to your branch / version.

**Example**: To use `jaredallard/feat/my-cool-feature` instead of `v1.8.0`, you'd update `versions.devbase` to the former. Full example below:

```yaml
# THIS IS AN AUTOGENERATED FILE. DO NOT EDIT THIS FILE DIRECTLY.
# vim: set syntax=yaml:
version: v7.4.2
generated: 2021-07-15T18:33:49Z
versions:
  devbase: jaredallard/feat/my-cool-feature
```

CI will now use that branch, to use it locally re-run any `make` command. **Note**: This will not automatically update locally when the remote branch is changed, in order to do that you will need to `rm -rf .bootstrap` and re-run a `make` command. If you are testing changes to `make e2e`, you must also run
`rm -rf ~/.outreach/.cache/gobin/binaries/$(go version | awk '{ print $3 }' | tr -d 'go')/github.com/getoutreach/devbase` before re-running a make command, in addition to `rm -rf .bootstrap`.

<!--- EndBlock(overview) -->
