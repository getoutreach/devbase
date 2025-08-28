# devbase

<!-- <<Stencil::Block(customGeneralInformation)>> -->
Contributions to `devbase` are super welcome! This document is a guide to help you get started.

Generally when working on larger scale features and changes, it's a good idea to open an RFC (Request For Comments) to discuss the design and implementation of the feature. This is to ensure that the design and implementation of the feature is well thought out, documented, and given time for feedback from the community.

To learn more about that, look at the documentation in the [`./rfcs`](./rfcs/) directory.
<!-- <</Stencil::Block>> -->

## Prerequisites

<!-- <<Stencil::Block(customPrerequisites)>> -->

### Before running

Due to `./scripts/bats/bats`, `./scripts/bats/test_helper/bats-assert` and `./scripts/bats/test_helper/bats-support` being *git submodules*, run:

```shell
git submodule update --init
```

<!-- <</Stencil::Block>> -->

## Building and Testing

This project uses devbase, which exposes the following build tooling: [devbase/docs/makefile.md](https://github.com/getoutreach/devbase/blob/main/docs/makefile.md)

<!-- <<Stencil::Block(customBuildingAndTesting)>> -->
### Shell Script Testing (with bats)

We use [bats](https://github.com/bats-core/bats-core) to test our shell scripts. You can run the tests with:

```bash
make test
```

To write a test, simply created a `<name>_test.bats` file with `<name>`
being replaced by the name of the shellscript you wish to test. A good
starting point is to model it based off of this template:

```bash
#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load <shellScriptHere>.sh

@test "function_goes_here should do xyz" {
  run function_goes_here
  assert_output "expected output"
}
```

For more information on how to write bats tests, see the [bats
documentation](https://bats-core.readthedocs.io/en/stable/writing-tests.html)
as well as existing tests in this repository (search for `*_test.bats` files!)

### Building and testing the CircleCI orb

If you want to test changes to the shared orb:

1. Make the changes to the orb
2. Run `mise run orb:publish-dev`
3. In a test service, change the orb version (after the `@`,
   [example](https://github.com/getoutreach/devbase/blob/8f298fa86e5ff37afc75f6c6eeda14275f758f25/.circleci/config.yml#L5))
   to `dev:first`

<!-- <</Stencil::Block>> -->

### Replacing a Remote Version of the a Package with Local Version

_This is only applicable if this repository exposes a public package_.

If you want to test a package exposed in this repository in a project that uses it, you can
add the following `replace` directive to that project's `go.mod` file:

```
replace github.com/getoutreach/devbase => /path/to/local/version/devbase
```

**_Note_**: This repository may have postfixed it's module path with a version, go check the first
line of the `go.mod` file in this repository to see if that is the case. If that is the case,
you will need to modify the first part of the replace directive (the part before the `=>`) with
that postfixed path.

### Linting and Unit Testing

You can run the linters and unit tests with:

```bash
make test
```
