# Declarative Configuration

## Table of Contents

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [Implementation Details](#implementation-details)
  - [Configuration example](#examples)
- [Test Plan](#testrelease-plan)
<!-- /toc -->


## Summary

This proposal replaces environment variable based configuration being used in Make+Magefiles in favor of using declarative yaml based configuration.

## Motiviation

As we've been writing `devbase`, and other tooling that uses it, we've identified that the current practice of environment variable based configuration as well as opinionated and unchangeable defaults is not flexible enough for using this tooling both outside of stencil-base (where it is primarily used today) and giving our users the flexibility to easily make minor behaviour changes. This has, ultimately created two pain points:

 * Documentation is hard to write and not easily discoverable
 * Finding configuration consumption, and configurable areas is difficult
 * Using `devbase` outside of `stencil-base` projects is not easy (must use sub-modules, and even then it's hard to "plug and play") 

### Goals

 - All configuration should be written as go structs (as we intend to only write new targets in), and have godoc compatible documentation on each exported field along with examples as needed.
 - Standard location for all configuration for tooling provided by devbase, et. al
 - Documentation on how to write Magefile targets, if the framework docs aren't self-explanatory enough
 - Ensure that all targets are "plug-and-play" compatible (e.g. usable by themselves), they shouldn't require stencil modules to be used, and if they do require certain options to work "out of the box" they should be sane defaults and well documented (as per the above goals)
 - Not breaking, while we could do a lot more if we did a breaking release, breaking _all_ of the existing releasing tooling isn't optimal

### Non-goals

 - Expose new functionality, this would be nice to have but would balloon the amount of work. We're targeting 1:1 compatibility with features/configuration already exposed today, with nice-to-haves being limited.
 - Migrating `Makefile` to `Magefile`, while it'd be nice to migrate all of them over, that's also an amount of work to do. While moving to `Magefile` would be good, we shouldn't try to move everything to Go at the same time (TL;DR: Wrap shell were needed).

## Proposal

The high level proposal here is generally covered in the [Summary](#summary) and [Moviation](#motiviation) sections.

### Implementation Details

#### Repository Configuration

Configuration would live in the root of repository consuming `devbase`, in `.devbase`.

Each configuration file would match the relevant `mage <target>` target, e.g.

```bash
$ ls .devbase
build.yaml
e2e.yaml
docker.yaml
```

**Note**: For shared configuration, e.g. `docker` (which could be shared between `docker-build` and `docker-push`) that's also allowed and can become a unified target.

#### Magefile Targets Implementation Details

Each magefile target that consumes this configuration will live inside of the `devbase` repository in the `targets/<targetName>` package, which will be pulled in by `root/mage.go`. The reasoning for a package per target is to make it easier to test these in isolation, as well as (generated) documentation to be localized to the packages (targets) themselves. A library will be provided at `pkg/targets`, which will provide functionality to read configuration and other shared functions between targets.

#### Magefile Targets Available Day One

The following targets will be available/configurable day one:

 - `build`
 - `debug`
 - `release`
 - `test` (includes: `coverage` as `--coverage`, `benchmark` as `--bench`)
 - `lint`
 - `fmt`
 - `e2e`
 - `docker` (includes: `build`, `publish`)

The rest will still be available for compatibility, but unconfigurable, e.g. `gobuild` will still build go but will not work for non-go repos.

See [Examples](#examples) for an idea of what options will be available/an idea of some of the steps.

#### Testing Framework

Testing would be done via Go Testing, enabling us to easily write unit tests. Integration tests would, unforuantely, still need to be figured out as they rely on being ran in macOS/Linux in a predictable fashion

### Examples

Below are example configuration objects that would be exposed, note this is not all-inclusive and is purposefully not fully spec'd out to allow for changes during implementation.

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

## Test/Release Plan

The test plan for this is to ensure that each target has unit, and where possible, integration tests. Releasing would follow the standard DT team releasing process, which is to be released in the next release window, while being put on the `rc` channel to be consumed by Outreach internally first.

