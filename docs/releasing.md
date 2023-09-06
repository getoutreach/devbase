# Releasing

## Unstable Releases

Unstable releases are created if the following options are set in a
`service.yaml`:

```yaml
arguments:
  releaseOptions:
    enablePrereleases: true
    # It is idiomatic to use 'rc' here.
    prereleasesBranch: <a-branch-other-than-the-default
```

When a PR is created to the default branch (normally `main`), it will be
tested for an unstable release. By default, if a repo has a
`.goreleaser.yml` file, binaries will be created an uploaded to the
`unstable` tag on the repo once merged.

If a repo does not have a `.goreleaser.yml` file, nothing will happen.
Optionally, a `scripts/unstable-release.include.sh` file can be created
that will be ran instead. If a `.goreleaser.yml` file does exist, and
the include file exists as well, it will be called after the github
release has been created.
