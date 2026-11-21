# Stencil Upgrade

The `shared/stencil_upgrade` orb job upgrades the Stencil modules of a repository. It runs on a schedule and opens a pull request with the result.

## What the job does

The job runs `mise run stencil:upgrade` in the repository. For repositories with a name that starts with `stencil-`, it runs `make` first.

When no upgrade pull request is open, the job works on a new branch from the default branch. If a module version changed, the job commits the changes to that branch and opens a pull request. If no module version changed, the job stops, unless `forcePR` applies. The job never opens an empty pull request.

## Opt in

Set these arguments for the `stencil-circleci` module in `service.yaml`. Only `scheduled` is required:

```yaml
arguments:
  stencilUpgrade:
    scheduled: true
```

- `stencilUpgrade.scheduled` is a boolean that controls the scheduled job. Running `stencil` by hand is not affected.
- `stencilUpgrade.cron` is a schedule in cron syntax, in UTC. The default is Thursday. The time of day comes from the service name, so repositories do not all run at the same time.
- `stencilUpgrade.forcePR` is a boolean. The default is `false`. When it is `true`, the job opens a pull request if generated files changed, even if no module version changed. The job also does this when `stencil.lock` has a module with the version `unstable`.

The job has no opt-in check of its own. The schedule exists only for repositories that opt in.

## The pull request

- The title is `fix(stencil): upgrade modules (<UTC date>)`.
- The branch is `stencil-upgrade/<UTC date>`.
- The description has a table of the changed modules, with a link to the repository of each module. It links the release notes of each new version, or the commit history for a module on `unstable`. A module that uses a local directory has no release notes link. For a module with more than 20 new versions, it links the newest 20 and counts the older ones.
- The pull request is not a draft. It has no labels, reviewers or assignees.

A person must approve the pull request, for compliance reasons. The job does not enable auto-merge.

## Open pull requests

Before it makes changes, the job looks for open pull requests.

- If a pull request from a branch that starts with `stencilbot/` is open, the job stops.
- If a pull request from a branch that starts with `stencil-upgrade/` is open, the job runs the upgrade on that branch.
  - If any file changed, even if only generated files changed, the job adds a commit and updates the title and description. Otherwise, the job logs that the pull request is already current.
  - If no module version differs from the default branch and `forcePR` does not apply, the job logs a warning that the pull request may be empty. The job does not close it.
- If a person pushed a commit to the branch of the open pull request, the job skips the pull request. The job never force-pushes.
- The job ignores pull requests from forks.
- If the branch for today exists on the remote, but has no open pull request, the job fails. A person must delete the branch or open a pull request for it.

## Manual runs

The pipeline parameter `stencil_upgrade` (boolean) starts the `manual-stencil-upgrade` workflow. Set it to `true` when you trigger a pipeline in the CircleCI web app or with the CircleCI API.

The job parameter `dryrun` (boolean, default `false`) makes a dry run. The workflows from `stencil-circleci` do not set it. To make a dry run, run the job with `dryrun: true` in a CircleCI config, for example on a test branch.

## Environment variables

The job passes `force_pr` and `dryrun` to the script as `STENCIL_UPGRADE_FORCE_PR` and `STENCIL_UPGRADE_DRYRUN`. The script `shell/ci/stencil-upgrade.sh` reads these variables:

- `STENCIL_UPGRADE_FORCE_PR` is `true` or `false`. The default is `false`.
- `STENCIL_UPGRADE_DRYRUN` is `true` or `false`. The default is `false`. When it is `true`, the script prints the title and description, and pushes nothing.
