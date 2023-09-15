# Releasing

## Unstable Releases

Unstable releases are created if the following options are set in a
`service.yaml`:

```yaml
arguments:
  releaseOptions:
    enablePrereleases: true
    # It is idiomatic to use 'rc' here.
    prereleasesBranch: <a-branch-other-than-the-default>
```

When a PR is created to the default branch (normally `main`), it will be
tested for an unstable release. By default, if a repo has a
`.goreleaser.yml` file, binaries will be created and uploaded to the
`unstable` tag on the repo once merged.

### Custom Release Logic

If a repo does not have a `.goreleaser.yml` file, nothing will happen by
default.

Optionally, a `scripts/unstable-release.include.sh` file can be created
that will be run instead. If both a `.goreleaser.yml` file and the
include file exist, it will be called after the GitHub release has
been created.

A `DRYRUN` environment variable is passed to the include script to
enable custom dry-run logic like the original release script.
