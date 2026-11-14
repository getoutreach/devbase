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
`.goreleaser.yml` file or a `build` key in the `release` section of
`scripts/devbase.yaml`, binaries will be created and uploaded to the
`unstable` tag on the repo once merged.

### Custom Release Logic

If a repo has neither, nothing will happen by default.

Optionally, a `scripts/unstable-release.include.sh` file can be created
that will be run instead. If both a `.goreleaser.yml` file and the
include file exist, it will be called after the GitHub release has
been created.

A `DRYRUN` environment variable is passed to the include script to
enable custom dry-run logic like the original release script.

## Release configuration in `scripts/devbase.yaml`

`devbase release` reads its configuration from the `release` section
of `scripts/devbase.yaml`. The section replaces `.releaserc.yaml` and
`.goreleaser.yml`, which `devbase release` reads when the section does
not exist. If the section and one of these files both exist,
`devbase release` fails. Unknown keys are errors.

### Keys

| Key | Default | Description |
|---|---|---|
| `branches` | Required | Exactly one release branch, and any number of prerelease branches. A prerelease branch sets `prerelease` to its identifier, such as `rc`. |
| `allowMajorVersions` | `false` | When `false`, a release with a new major version fails. |
| `rules` | See below | Custom commit rules. Each rule has `type`, `scope` and `subject` globs, `breaking`, `revert`, and `release` (`patch`, `minor`, `major` or `none`). A commit that matches no custom rule uses the semantic-release commit analyzer rules, where `feat` is minor and `fix` is patch. |
| `build` | No build | The Go binaries and the `tar.gz` archives. |
| `publishers` | `[notes, github]` | What the release makes and publishes, in order. |
| `root` | None | The path rules of the root release when there are units. |
| `units` | None | Extra release units, each with its own tags. |

The default `rules` are `{breaking: true, release: minor}`,
`{type: revert, release: patch}` and `{type: perf, release: patch}`.
With `allowMajorVersions: true`, the default has no breaking rule.

The `build` key has these keys:

| Key | Default | Description |
|---|---|---|
| `unstableOnly` | `false` | When `true`, only the unstable release builds. The release builds nothing. |
| `before` | `[make dep]` | Commands to run before the build. |
| `builds` | Required | The Go programs. Each build has `binary`, `id` (default: the binary), `main` (default: `./cmd/<binary>`), `ldflags` (default: see below) and `env` (default: `[CGO_ENABLED=0]`). |
| `targets` | `[linux/amd64, linux/arm64, darwin/arm64]` | The `os/arch` pairs to build. |
| `archive.files` | `license*`, `LICENSE*`, `readme*`, `README*`, `changelog*`, `CHANGELOG*` | Extra files in each archive. Each file has `src`, and optional `dst` and `mode`. |
| `checksums` | `checksums.txt` | The checksum file name. |

The default `ldflags` set the version, and set `main.HoneycombTracingKey`
and `main.TeleforkAPIKey` from the `HONEYCOMB_APIKEY` and
`TELEFORK_APIKEY` environment variables. If one of these variables is
not set, the build fails. If a program does not use these keys, set its
`ldflags`.

The publishers are:

* `notes`: release notes from the conventional commits.
* `github`: the GitHub release. `assets` defaults to the archives and
  the checksum file when the release builds.
* `npm`: the npm package in `packageRoot` (default: the repository
  root).
* `gem`: the Ruby client gem, with `dir`, `name` and `versionFile`.
* `python`: the Python client on AWS CodeArtifact. `domain`, `region`
  and `repository` are required.
* `hooks`: shell commands for the `generateNotes`, `prepare`, `publish`
  and `success` steps.
* `commit`: the release commit. `files` defaults to `CHANGELOG.md`,
  `package.json`, `package-lock.json` and `npm-shrinkwrap.json`. The
  commit never includes the client version files.
* `orb`: the CircleCI orb `name`. `path` defaults to `orb.yml`.

### Example

A CLI with release candidates on `main`, which publishes an npm client:

```yaml
release:
  branches:
    - name: main
      prerelease: rc
    - name: release
  build:
    builds:
      - binary: widgets
  publishers:
    - notes
    - github
    - npm:
        packageRoot: api/clients/node
```

### Units

A unit is a part of the repository with its own tags. Each unit has a
`kind` (`go-module`, `npm` or `generic`) and a `dir`. A Go module unit
gets the Go tags of its folder, such as `api/v1.2.0`. By default, it
gets the version of the root release (`strategy: mirror`). The other
kinds need a `tagFormat`, and by default compute their own versions
(`strategy: independent`). An `npm` unit cannot use `mirror`. The
optional unit keys are `name`, `paths`, `exclude`, `strategy` (`mirror`
or `independent`), `skipPrereleases`, `allowMajorVersions`,
`forgeRelease`, `npm.packageRoot`, `npm.install` and `go.pinRoot`
(`auto` or `never`).

```yaml
release:
  branches:
    - name: main
  root:
    exclude: [node/**]
  units:
    - kind: go-module
      dir: api
    - kind: npm
      dir: node/widgets-client
      tagFormat: nodejs-widgets-client-v${version}
```

`root.exclude` lists path globs whose changes never count for the root
release. A change in `root.shared` counts for the root release only
when the commit changes no unit.

### Convert the old files

`devbase release config convert` prints the equivalent `release`
section to stdout, and notes to stderr. Put the section in
`scripts/devbase.yaml` and delete both old files. If stencil renders
`scripts/devbase.yaml` and no module adds the `release` section, put the
section between the `## <<Stencil::Block(release)>>` and
`## <</Stencil::Block>>` lines.

* A `.goreleaser.yml` without the build hook in `.releaserc.yaml`
  becomes a build with `unstableOnly: true`.
* An exec hook that runs `release-submodules.sh` becomes a `go-module`
  unit for each folder with a `go.mod` that the script names. A folder
  with its own `.releaserc.yaml` gets `strategy: independent`. The
  notes list the files to delete.
* The command does not add units for other Go modules or npm packages
  in subfolders. The notes list their folders.
