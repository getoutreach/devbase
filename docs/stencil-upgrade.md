# Stencil Upgrade

The `shared/stencil_upgrade` orb job upgrades the Stencil modules of a repository. It runs on a schedule and opens a pull request with the result.

## What the job does

The job runs `mise run stencil:upgrade` in the repository. For repositories with a name that starts with `stencil-`, it runs `make` first.

If a module version changed, the job commits the changes to a new branch and opens a pull request. If no module version changed, the job stops, unless `forcePR` applies. The job never opens an empty pull request.

## Opt in

Set these arguments for the `stencil-circleci` module in `service.yaml`. Only `scheduled` is required:

```yaml
arguments:
  stencilUpgrade:
    scheduled: true
```

- `stencilUpgrade.scheduled` is a boolean. When it is `true`, stencil writes the scheduled workflow. It controls only the scheduled job. Running `stencil` by hand is not affected.
- `stencilUpgrade.cron` is a schedule in cron syntax, in UTC. The default is Thursday. The time of day comes from the service name, so repositories do not all run at the same time.
- `stencilUpgrade.forcePR` is a boolean. The default is `false`. When it is `true`, the job opens a pull request if generated files changed, even if no module version changed. The job also does this when `stencil.lock` has a module with the version `unstable`.

The job has no opt-in check of its own. The schedule exists only for repositories that opt in.

## The pull request

- The title is `fix(stencil): upgrade modules (<UTC date>)`.
- The branch is `stencil-upgrade/<UTC date>`.
- The description links to the upgraded versions of each module.
- The pull request is not a draft. It has no labels and no reviewers.

A person must approve the pull request, for compliance reasons. The job does not enable auto-merge.

## Open pull requests

Before it makes changes, the job looks for open pull requests.

- If a pull request from a branch that starts with `stencilbot/` is open, the job stops.
- If a pull request from a branch that starts with `stencil-upgrade/` is open, the job adds a commit to that branch and updates the title and description. If a person pushed a commit to the branch, the job skips the pull request. The job never force-pushes.
- If the branch for today exists on the remote, but has no open pull request, the job fails. A person must look at the branch.

## Environment variables

The job passes `force_pr` to the script as `STENCIL_UPGRADE_FORCE_PR`. The script `shell/ci/stencil-upgrade.sh` reads these variables:

- `STENCIL_UPGRADE_FORCE_PR` is `true` or `false`. The default is `false`.
- `STENCIL_UPGRADE_DRYRUN` is `true` or `false`. The default is `false`. When it is `true`, the script prints the title and description, and pushes nothing.
