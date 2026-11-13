# CODEOWNERS linter

`devbase lint codeowners` validates the [CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners) file of a repository.

This command is derived from [mszostok/codeowners-validator](https://github.com/mszostok/codeowners-validator) (Apache License 2.0).

## Usage

Run the command in the repository:

```shell
devbase lint codeowners
```

The command uses the `CODEOWNERS` file in the root, `docs/`, or `.github/` directory of the repository. Without `--repository-path`, the root of the repository is the root of the git work tree of the current directory. Outside of a git repository, it is the current directory. When git fails for another reason, for example because another user owns the repository and `--trust-workspace` is not set, the command fails.

The command runs the checks in parallel and prints the results in the order of `--checks`. Run `devbase lint codeowners --help` to list all flags. The general flags are:

| Flag                    | Default                           | Description                                                                                            |
| ----------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `--repository-path`     | The root of the git work tree     | Path to the root of the repository.                                                                    |
| `--checks`              | `files,owners,duppatterns,syntax` | Checks to run. Repeat the flag or separate the names with commas. The value replaces the default list. |
| `--check-failure-level` | `warning`                         | With `warning`, warnings and errors make the command fail. With `error`, only errors make it fail.     |
| `--repository`          | The `origin` remote               | GitHub repository in the form `owner/repository`, for the `owners` check.                              |
| `--staged`              | `false`                           | Check the files in the git index only. See [Pre-commit hook](#pre-commit-hook).                        |
| `--trust-workspace`     | `false`                           | Make git trust the repository directory, even when another user owns it.                               |

To run all the checks:

```shell
devbase lint codeowners --checks files,owners,duppatterns,syntax,notowned,avoid-shadowing
```

To run the default checks without GitHub access:

```shell
devbase lint codeowners --checks files,duppatterns,syntax
```

## Pre-commit hook

In a git pre-commit hook, use `--staged`, so that the command checks what the commit will contain. For example, to run the checks that need no GitHub access:

```shell
devbase lint codeowners --staged --checks files,duppatterns,syntax,notowned
```

With `--staged`:

- The command reads the CODEOWNERS file from the git index. Without `--staged`, it reads the file from the working tree.
- The `files` check uses only the files in the git index. Without `--staged`, it also uses the untracked files that git does not ignore, and it does not use the files deleted from the working tree.

The `notowned` check always uses the files in the git index.

## Exit codes

The command exits with 1 when a check finds an issue at or above `--check-failure-level`, or when a check cannot complete, for example because a GitHub API call fails. It also exits with 1 when it cannot start, for example when a flag value is not valid, or when it finds no CODEOWNERS file or more than one. Otherwise, it exits with 0.

## Checks

| Name              | Default | Description                                                                                                                                                                                                                                                                         |
| ----------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `files`           | Yes     | Reports each pattern that matches no file. The check uses the files that git tracks, except the files deleted from the working tree, and the untracked files that git does not ignore. With `--staged`, it uses only the files in the git index. A submodule counts as a directory. |
| `owners`          | Yes     | Reports each owner that is not valid. This check needs GitHub access. See [The `owners` check](#the-owners-check).                                                                                                                                                                  |
| `duppatterns`     | Yes     | Reports each pattern that is on more than one line.                                                                                                                                                                                                                                 |
| `syntax`          | Yes     | Reports each owner that does not look like a GitHub username, a team name, or an email address.                                                                                                                                                                                     |
| `notowned`        | No      | Reports each file in the git index that no pattern matches. This check is experimental.                                                                                                                                                                                             |
| `avoid-shadowing` | No      | Reports each pattern that shadows an earlier pattern. The last matching pattern takes precedence, so put the patterns in order from the least specific to the most specific. This check is experimental.                                                                            |

### The `owners` check

An owner is a user (`@username`), a team (`@org/team-name`), or an email address. The check validates users and teams as follows:

- A user must be a collaborator of the repository with write access (`push`, `maintain`, or `admin`). When GitHub does not return the permissions of a collaborator, the check uses the role of the collaborator instead: `write`, `maintain`, or `admin`.
- A team must belong to the organization of the repository and have write access to the repository, directly or through a parent team.

The check does not look up email addresses on GitHub. It compares user logins, organization names, team slugs, and ignored owners without regard to case. It reports an owner that is not valid only on the first line where the owner appears.

| Flag                       | Default  | Description                                                                                     |
| -------------------------- | -------- | ----------------------------------------------------------------------------------------------- |
| `--ignored-owners`         | `@ghost` | Owners that the check does not validate.                                                        |
| `--allow-unowned-patterns` | `true`   | Allow patterns without owners. Use `--allow-unowned-patterns=false` to report them as warnings. |
| `--owners-must-be-teams`   | `false`  | Require every owner to be a team of the organization of the repository.                         |

### Flags for the `notowned` check

| Flag               | Default | Description                                                                 |
| ------------------ | ------- | --------------------------------------------------------------------------- |
| `--skip-patterns`  | None    | Patterns that do not count as owning a file, such as a default `*` pattern. |
| `--subdirectories` | None    | Directories to limit the check to.                                          |

Each `--subdirectories` value is a git pathspec (see `gitglossary(7)`), so it can also be a file or a glob. The check fails when a value matches no file in the git index.

## GitHub access

Only the `owners` check needs GitHub access. The command looks for credentials only when this check runs. `--github-timeout` sets the timeout of each GitHub API request. The default is 30 seconds.

### Access token

The command uses the first token that it finds in this order:

1. The `GITHUB_TOKEN` environment variable.
2. The `OUTREACH_GITHUB_TOKEN` environment variable.
3. The file `~/.outreach/github.token`.
4. The output of `gh auth token`.

If it finds no token, it runs `gh auth login` when `gh auth status` fails. Then it reads the token from `~/.config/gh/hosts.yml`.

### Required access

Before any check runs, the command gets the repository to confirm that the credentials work. The `owners` check then calls the other endpoints in this table:

| Endpoint                                                 | Used for                                            | Scopes (classic or OAuth app token)              | Permissions (fine-grained token or GitHub App)                 |
| -------------------------------------------------------- | --------------------------------------------------- | ------------------------------------------------ | -------------------------------------------------------------- |
| `GET /repos/{owner}/{repo}`                              | Confirm access                                      | `repo`, for a private repository                 | Repository "Metadata" (read)                                   |
| `GET /repos/{owner}/{repo}/collaborators`                | Users                                               | `repo` and `read:org`                            | Repository "Metadata" (read)                                   |
| `GET /repos/{owner}/{repo}/teams`                        | Teams (optional)                                    | `repo`, or `public_repo` for a public repository | Repository "Administration" (read)                             |
| `GET /orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}` | Other teams, including access through a parent team | `repo` or `admin:org`, for a private repository  | Organization "Members" (read) and repository "Metadata" (read) |

The list of the repository teams is optional. When GitHub refuses it with HTTP 403, for example because a GitHub App does not have the repository permission "Administration" (read), the check uses one team repository call (`GET /orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}`) for each team.

A scope that includes a listed scope also works. For example, `write:org` and `admin:org` include `read:org`. To list the collaborators, the authenticated user must also have write, maintain, or admin privileges on the repository. For a repository that an organization owns, the user must also be a member of the organization.

When a call fails because of missing access, the error message names the access that the endpoint needs. For a classic token, it also names the missing scopes.

### GitHub App

To authenticate as a GitHub App installation, set the App ID, the installation ID, and the private key of the App in PEM format:

| Flag                           | Environment variable         |
| ------------------------------ | ---------------------------- |
| `--github-app-id`              | `GITHUB_APP_ID`              |
| `--github-app-installation-id` | `GITHUB_APP_INSTALLATION_ID` |
| None                           | `GITHUB_APP_PRIVATE_KEY`     |

When the App ID is set, the command does not look for an access token.

### GitHub Enterprise Server

Set `--github-base-url` to the API base URL of the server, and `--github-upload-url` to its upload URL. The upload URL defaults to the base URL. The command does not get the server from the `origin` remote. Without `--github-base-url`, it uses the API of github.com.

`gh auth token` gives the token of the default host of `gh`, because the command does not give it `--hostname`. To use a token for the server, set `GITHUB_TOKEN` to the token, or set `GH_HOST` to the hostname of the server.

## Differences from codeowners-validator

- The `notowned` check does not change the repository. It lists the files in the git index, so it works with uncommitted changes and ignores untracked files. The upstream check reports an issue when `git status` shows changes. Otherwise, it appends the patterns to `.gitignore`, runs `git rm --cached` on the ignored files, and then runs `git reset --hard`.
- The `owners` check requires a user to be a collaborator of the repository with write access. The upstream check requires a user to be a member of the organization.
- The upstream command checks that a token has the `read:org` scope. This command does not check scopes, because fine-grained tokens and GitHub Apps use permissions instead of scopes.
- The command takes its settings from flags, except for some authentication settings (see [GitHub access](#github-access)). `--checks` selects both the default and the experimental checks. The upstream command uses environment variables, with `CHECKS` and `EXPERIMENTAL_CHECKS` for the checks.
- `--trust-workspace` passes the repository directory as `safe.directory` to each git command. To find the root of the git work tree, it passes `*`, because the root is not known yet. The upstream command adds the directory to the global git configuration.
- When a check fails, the command exits with 1. The upstream command exits with 3.
