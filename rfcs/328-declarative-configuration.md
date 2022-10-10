# RFC-328: Declarative Configuration

## Table of Contents

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-goals](#non-goals)
- [Design Details](#design-details)
  - [Repository Configuration](#repository-configuration)
  - [Magefile Targets Implementation Details](#magefile-targets-implementation-details)
  - [Magefile Targets](#magefile-targets)
  - [Testing Framework](#testing-framework)
  - [Configuration Examples](#configuration-examples)
- [Testing and Release Plan](#testing-and-release-plan)
- [Implementation History](#implementation-history)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
<!-- /toc -->

## Summary

This proposal introduces a system for defining configuration for all of the current make targets provided by `devbase` in a declarative format. This format will be backed by Go structs and stored as YAML within each repository that uses these make targets. Also covered is the in-flight work of migrating the current make targets to a Go-based `mage` build system instead.

## Motivation

As we've been writing `devbase` and other tooling that uses it, we've identified that the current method of configuration using environment variables with opinionated and unchangeable defaults is not flexible enough for using `devbase` outside of stencil-base and giving our users the flexibility to make minor behavioral changes. This is summarized in three pain points:

- Documentation is hard to write and not easily discoverable.
- Finding where configuration is used and what is configurable is difficult.
- Using `devbase` outside of `stencil-base` projects is not easy. Users must use sub-modules, and even then it's hard to "plug and play" with existing projects' CI and local build systems.

### Goals

- All configuration should be written as go structs (as we intend to only write new targets in Go), and have godoc compatible documentation on each exported field along with examples as needed.
- Standard location for all configuration for tooling provided by devbase, et al.
- Documentation on how to write Magefile targets.
- Ensure that all targets are "plug-and-play" compatible (e.g. usable by themselves), they shouldn't require stencil modules to be used, and if they do require certain options to work "out of the box" they should be sane defaults and well documented (as per the above goals).
- Not breaking (backwards-compatible). While we could do a lot more if we did a breaking release, breaking _all_ of the existing releasing tooling isn't optimal.

### Non-goals

- Expose new functionality. This would be nice to have but would balloon the amount of work. We're targeting 1:1 compatibility with features/configuration already exposed today, with nice-to-haves being limited.
- Migrating `Makefile` to `Magefile`. While it'd be nice to migrate all of them over, that's also an amount of work to do. While moving to `Magefile` would be good, we shouldn't try to move everything to Go at the same time (TL;DR: Wrap shell where needed).

## Design Details

The overarching design is to move all of the current environment variable based config to be backed by YAML, and strongly-typed through Go structs, consumed by `mage` targets that replace the currently existing `make` targets. This will allow us to have a single source of truth for configuration, and allow us to have a single place to document all of the configuration options as well as "for free" documentation rendered on [pkg.go.dev](https://pkg.go.dev).

### Repository Configuration

Configuration will live in the root of repository consuming `devbase`, in the `.devbase` folder.

Each configuration file will match the relevant `make <target>` target with a `.yaml` suffix added, e.g.

```bash
$ ls .devbase
build.yaml
e2e.yaml
docker.yaml
```

**Note**: Configuration should be able to be shared between targets (e.g. `docker-build` and `docker-push` today largely share `docker.yaml`), but this should be limited to only being able to consume the same configuration struct, not having two different configurations live in one file. The idea is to allow _closely_ related targets to consume the same configuration.

### Magefile Targets Implementation Details

Each Magefile target that consumes this configuration will live inside of the `devbase` repository in the `targets/<targetName>` package, which will be pulled in by `root/mage.go`. The reasoning for a package per target is to make it easier to test these in isolation, as well as (generated) documentation to be localized to the packages (targets) themselves. A library will be provided at `pkg/targets`, which will provide functionality to read configuration and other shared functions between targets.

### Magefile Targets

The following targets are examples/ideas of what we'd like to have available in `devbase` as targets:

- `build`
- `debug`
- `release`
- `test` (includes: `coverage` as `--coverage`, `benchmark` as `--bench`)
- `lint`
- `fmt`
- `e2e`
- `docker` (includes: `build`, `publish` as subcommands)

The other existing targets will still be available for compatibility, but will not be configurable, e.g. `gobuild` will still build go but will not work for non-go repos.

See [Configuration Examples](#configuration-examples) for an idea of what options will be available/an idea of some of the steps.

### Testing Framework

Testing would be done via Go Testing, enabling us to easily write unit tests. Integration tests would, unfortunately, still need to be figured out as they rely on being run in macOS/Linux in a predictable fashion

### Configuration Examples

Below are example configuration objects that would be exposed. Note this is not all-inclusive and is purposefully not fully spec'd out to allow for changes during implementation.

An example configuration might look like so, for `build`:

```yaml
golang:
  dir: ./cmd/...
  ldflags:
    github.com/getoutreach/gobox/pkg/app.Version: '{{ .Version }}'
```

Or `docker.yaml`, that already [exists today](https://github.com/getoutreach/devbase#building-docker-images):

```yaml
# Corresponds to the directory in `deployments/` and is used, by default, as the image name (see special case below).
# If this is equal to "appName" then the build context/image name is changed. See docs.
image_name:
  # Override the build context for the image. Defaults to '.' if image_name == appName, otherwise ./deployments/<image_name>
  buildContext: .
  # pushTo is a different image registry to push to, defaults to:
  # (box: .devenv.imageRegistry)/$image (or, if not == appName, $appname/$image)
  pushTo: docker.com/stable/athens
  # extra secrets to expose to the builder, defaults to NPM_TOKEN being exposed
  # See: https://docs.docker.com/develop/develop-images/build_enhancements/#new-docker-build-secret-information
  secrets:
    - id=mySecret,env=MY_SECRET_ENV_VARIABLE
  # Platforms to build this image for, defaults to linux/amd64,linux/arm64
  # See: https://github.com/docker/buildx#building-multi-platform-images
  platforms:
    - linux/amd64
```

## Testing and Release Plan

Testing will be figured out more as we get closer to implementation, but the general idea is to have unit tests for each target, and integration tests for targets that require a full environment to be ran in (e.g. `docker`) using the framework that will be designed during the implementation phase.

## Implementation History

- 2022-09-26: Initial draft

## Drawbacks

Moving to declarative configuration presents challenges with adding flexibility from a user perspective.
Users cannot construct their own chain of logic, and are instead limited to the functionality provided by
the configuration format.

We consider this to be acceptable, however, as one of the main design principles of devbase is to take an
opinionated stance on how to build software, and to provide a consistent experience across all repositories.

## Alternatives

A potential alternative would be to build building blocks for users to do these operations and allow users
to chain them together for the behaviour they want, instead of providing a declarative configuration format.

However, this would make it more complicated to provide a consistent experience across all repositories as well
as make it more difficult to maintain. This would also make it harder to provide out of the box functionality
for common tasks, such as building docker images in a DRY way.
