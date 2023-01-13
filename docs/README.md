# devbase

devbase is a collection of tools and scripts to help you get started with a new project.

## Getting Started

To get started, you'll need to use [stencil](https://github.com/getoutreach/stencil) and add `devbase` as a module:

```yaml
# service.yaml
name: my-cool-app
modules:
  - name: github.com/getoutreach/devbase
```

From there you can run `stencil`, you'll now be able to start using `devbase`!

## What's Included

* Common `make` targets for things like building, testing, linting, etc.
* Docker image building
* E2E test runner using Kubernetes
* Ruby package publishing
* Documentation publishing pipeline (to Confluence)
* Automatic version bumping using [Semantic Versioning](https://semver.org/) and [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
* A bunch of other stuff

## Documentation

* [Makefile Targets](./makefile.md)

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for more information.
