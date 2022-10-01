# devbase

<!-- <<Stencil::Block(customGeneralInformation)>> -->
Contributions to `devbase` are super welcome! This document is a guide to help you get started.

Generally when working on larger scale features and changes, it's a good idea to open an RFC (Request for Comments) to discuss the design and implementation of the feature. This is to ensure that the design and implementation of the feature is well thought out, documented, and given time for feedback from the community.

To learn more about that, look at the documentation in the [`./rfcs`](./rfcs/) directory.
<!-- <</Stencil::Block>> -->

## Prerequisites

<!-- <<Stencil::Block(customPrerequisites)>> -->

<!-- <</Stencil::Block>> -->

## Building and Testing

<!-- <<Stencil::Block(customBuildingAndTesting)>> -->

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
