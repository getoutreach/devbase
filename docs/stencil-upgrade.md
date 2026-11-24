# Stencil Upgrade

The `shared/stencil_upgrade` orb job upgrades the Stencil modules of a repository and opens a pull request with the result. The `stencil-circleci` module adds a workflow that runs the job on demand, and a weekly schedule for repositories that opt in. The [module documentation](https://github.com/getoutreach/stencil-circleci/blob/main/docs/arguments.md) describes the arguments and the manual trigger.

## What the job does

The job runs `mise run stencil:upgrade` in the repository. For repositories with a name that starts with `stencil-`, it runs `make` first.

When no upgrade pull request is open, the job works on a new branch from the default branch. If a module version changed, the job commits the changes to that branch and opens a pull request. If no module version changed, the job stops, unless `force_pr` applies. The job never opens an empty pull request.

## Parameters

- `force_pr` (boolean, default `false`) makes the job open a pull request if generated files changed, even if no module version changed. The job also treats `force_pr` as `true` when `stencil.lock` has a module with the version `unstable`.
- `note` (string, default empty) adds Markdown at the top of the pull request description, above the table of changes. Use it to explain a change that the module versions do not show, for example a move to another vendor. A run that changes the branch of an open pull request replaces the description, so the note stays only if that run sets it again.
- `dryrun` (boolean, default `false`) prints the title and description of the pull request that the job would open or update, and pushes nothing. The `stencil-circleci` workflows do not set it. To make a dry run, run the job with `dryrun: true` in a CircleCI config, for example on a test branch.

The job also takes `machine`, `resource_class`, `executor`, `no_output_timeout` and `setup_remote_docker`. They configure the job environment. See `orbs/shared/jobs/stencil_upgrade.yaml` for the defaults.

The job passes the parameters to `shell/ci/stencil-upgrade.sh` as `STENCIL_UPGRADE_FORCE_PR`, `STENCIL_UPGRADE_NOTE` and `STENCIL_UPGRADE_DRYRUN`. The script runs `stencil_upgrade_run` from `shell/lib/stencil_upgrade.sh`, which reads them.

## The pull request

- The title is `fix(stencil): upgrade modules (<UTC date>)`.
- The branch is `stencil-upgrade/<UTC date>`.
- The description has a table of the changed modules, with a link to the repository of each module. It links the release notes of each new version, or the commit history for a module on `unstable`. A module that uses a local directory has no release notes link. For a module with more than 20 new versions, it links the newest 20 and counts the older ones.
- The pull request is not a draft. It has no labels, reviewers or assignees.

A person must approve the pull request, for compliance reasons. The job does not enable auto-merge.

## Open pull requests

Before it makes changes, the job looks for open pull requests.

- If a pull request from a branch that starts with `stencilbot/` is open, the job logs a warning and stops.
- If a pull request from a branch that starts with `stencil-upgrade/` is open, the job runs the upgrade on that branch.
  - If any file changed, even if only generated files changed, the job adds a commit and updates the title and description. Otherwise, the job logs that the pull request is already current.
  - If no module version differs from the default branch and `force_pr` does not apply, the job logs a warning that the pull request may be empty. The job does not close it.
- If a person pushed a commit to the branch of the open pull request, the job skips the pull request. The job never force-pushes.
- The job ignores pull requests from forks.
- If the branch for today exists on the remote, but has no open pull request, the job fails. A person must delete the branch or open a pull request for it.
