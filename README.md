# devbase
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/getoutreach/devbase)
[![Generated via Bootstrap](https://img.shields.io/badge/Outreach-Bootstrap-%235951ff)](https://github.com/getoutreach/bootstrap)
[![Coverage Status](https://coveralls.io/repos/github/getoutreach/devbase/badge.svg?branch=main)](https://coveralls.io/github//getoutreach/devbase?branch=main)
<!-- <<Stencil::Block(extraBadges)>> -->

<!-- <</Stencil::Block>> -->

A collection of scripts and ci configuration

## Contributing

Please read the [CONTRIBUTING.md](CONTRIBUTING.md) document for guidelines on developing and contributing changes.

## High-level Overview

<!-- <<Stencil::Block(overview)>> -->

## How to use a Custom Build of `devbase`

### Stencil

To test a custom build of `devbase` with stencil, simply modify `stencil.lock` to point to your branch / version.

**Example**: To use `jaredallard/feat/my-cool-feature` instead of `v1.8.0`, you'd update `modules` entry for `devbase` as shown below: 

```yaml
...
modules:
    - name: github.com/getoutreach/devbase
      url: https://github.com/getoutreach/devbase
      version: jaredallard/feat/my-cool-feature
...
```

CI will now use that branch, to use it locally re-run any `make` command. **Note**: This will not automatically update locally when the remote branch is changed, in order to do that you will need to `rm -rf .bootstrap` and re-run a `make` command. If you are testing changes to `make e2e`, you must also run
`rm -rf ~/.outreach/.cache/gobin/binaries/$(go version | awk '{ print $3 }' | tr -d 'go')/github.com/getoutreach/devbase` before re-running a make command, in addition to `rm -rf .bootstrap`.

## Building Docker Images

Docker images can be built by creating a `docker.yaml` file in the `deployments/` directory of a repository. The format of this file is as follows:

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

<!-- <</Stencil::Block>> -->
