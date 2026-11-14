# CODEOWNERS linter

`devbase lint codeowners` validates the [CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners) file of a repository.

This command is derived from [mszostok/codeowners-validator](https://github.com/mszostok/codeowners-validator) (Apache License 2.0).

## Usage

Run the command in the repository:

```shell
devbase lint codeowners
```

The command uses the `CODEOWNERS` file in the root, `docs/`, or `.github/` directory of the repository. When that file is a symbolic link, the command fails, with and without `--staged`. Without `--repository-path`, the root of the repository is the root of the git work tree of the current directory. Outside of a git repository, it is the current directory. When git fails for another reason, for example because another user owns the repository and `--trust-workspace` is not set, the command fails.

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

## CI

When a repository has a CODEOWNERS file, `make lint` runs the CODEOWNERS linter, `shell/linters/codeowners.sh`. The linter runs:

```shell
devbase lint codeowners --checks files,duppatterns,syntax,owners --owners-if-changed --base <default branch>
```

The default branch is the branch that `origin/HEAD` points to, for example `origin/main`. When `origin/HEAD` is not set, it is `main`.

The `owners` check makes GitHub API calls. All the CI jobs that use the same token share its rate limit. To use fewer requests:

- The check runs only when CODEOWNERS changed since the merge base with the default branch. On the default branch, it runs only when CODEOWNERS changed since the first parent of `HEAD`. See [Only when CODEOWNERS changed](#only-when-codeowners-changed).
- When the command cannot read the commit to compare with, for example in a shallow clone, the check runs on each build, and the command prints a warning.
- The check is skipped when fewer than 500 requests remain in the rate limit. See [Rate limit](#rate-limit).

The linter uses the token in `GITHUB_TOKEN`. In CI, the linter framework uses `GITHUB_TOKEN` when it is set, or else gets a token with `ghaccesstoken`. Outside of CI, it uses the output of `gh auth token`. When the token is a GitHub App installation token, the App needs the repository permission "Metadata" (read) and the organization permission "Members" (read). The repository permission "Administration" (read) is optional. See [Required access](#required-access).

## Exit codes

The command exits with 1 when a check finds an issue at or above `--check-failure-level`, or when a check cannot complete, for example because a GitHub API call fails. It also exits with 1 when it cannot start, for example when a flag value is not valid, or when it finds no CODEOWNERS file or more than one. Otherwise, it exits with 0.

A skipped check does not make the command fail. See [Only when CODEOWNERS changed](#only-when-codeowners-changed) and [Rate limit](#rate-limit).

## Checks

| Name              | Default | Description                                                                                                                                                                                                                                                                                                    |
| ----------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `files`           | Yes     | Reports each pattern that matches no file. See [Patterns](#patterns). The check uses the files that git tracks, except the files deleted from the working tree, and the untracked files that git does not ignore. With `--staged`, it uses only the files in the git index. A submodule counts as a directory. |
| `owners`          | Yes     | Reports each owner that is not valid. This check needs GitHub access. See [The `owners` check](#the-owners-check).                                                                                                                                                                                             |
| `duppatterns`     | Yes     | Reports each pattern that is on more than one line.                                                                                                                                                                                                                                                            |
| `syntax`          | Yes     | Reports each owner that does not look like a GitHub username, a team name, or an email address. It also reports a warning for each pattern that starts with `!` (negation) or `\#` (an escaped `#`). CODEOWNERS does not support these features.                                                               |
| `notowned`        | No      | Reports each file in the git index that has no owner: no pattern matches the file, or the last pattern that matches it has no owners. See [Patterns](#patterns). A submodule counts as a directory. This check is experimental.                                                                                |
| `avoid-shadowing` | No      | Reports each pattern that shadows an earlier pattern. The last matching pattern takes precedence, so put the patterns in order from the least specific to the most specific. This check is experimental.                                                                                                       |

### The `owners` check

An owner is a user (`@username`), a team (`@org/team-name`), or an email address. The check validates users and teams as follows:

- A user must be a collaborator of the repository with write access (`push`, `maintain`, or `admin`). When GitHub does not return the permissions of a collaborator, the check uses the role of the collaborator instead: `write`, `maintain`, or `admin`.
- A team must belong to the organization of the repository and have write access to the repository, directly or through a parent team.

The check does not look up email addresses on GitHub. It compares user logins, organization names, team slugs, and ignored owners without regard to case. It reports an owner that is not valid only on the first line where the owner appears.

| Flag                       | Default  | Description                                                                                                    |
| -------------------------- | -------- | -------------------------------------------------------------------------------------------------------------- |
| `--ignored-owners`         | `@ghost` | Owners that the check does not validate.                                                                       |
| `--allow-unowned-patterns` | `true`   | Allow patterns without owners. Use `--allow-unowned-patterns=false` to report them as warnings.                |
| `--owners-must-be-teams`   | `false`  | Require every owner to be a team of the organization of the repository.                                        |
| `--owners-if-changed`      | `false`  | Run the check only when CODEOWNERS changed. See [Only when CODEOWNERS changed](#only-when-codeowners-changed). |
| `--base`                   | `main`   | The ref for `--owners-if-changed`.                                                                             |

#### Only when CODEOWNERS changed

With `--owners-if-changed`, the command compares the CODEOWNERS file that it loaded with the file at the same path in an earlier commit. The loaded file is in the working tree, or in the git index with `--staged`. The earlier commit is the merge base of `HEAD` and `--base`. When the merge base is `HEAD` itself, for example on the `--base` branch, the earlier commit is the first parent of `HEAD`.

When the two files are the same, the command skips the `owners` check, and it does not look for GitHub credentials. The check runs when the files are different, when the file does not exist in the earlier commit, or when `HEAD` has no parent.

When the command cannot read the earlier commit, for example in a shallow clone or when `--base` does not exist, the check runs, and the command prints a warning.

On the `--base` branch, a squash merge or a merge commit differs from its first parent by all the changes of its pull request. Thus the check also validates a change to CODEOWNERS whose check was skipped in the pull request, for example because of the [rate limit](#rate-limit).

An owner can become invalid without a change to CODEOWNERS, for example when a user leaves the organization. With `--owners-if-changed`, the check does not find this issue until the next change to CODEOWNERS.

### Flags for the `notowned` check

| Flag               | Default | Description                                                     |
| ------------------ | ------- | --------------------------------------------------------------- |
| `--skip-patterns`  | None    | Patterns that the check ignores, such as a default `*` pattern. |
| `--subdirectories` | None    | Directories to limit the check to.                              |

The check removes the lines whose pattern is a `--skip-patterns` value before it finds the last matching line of each file, so a file can fall back to an earlier matching line.

Each `--subdirectories` value is a git pathspec (see `gitglossary(7)`), so it can also be a file or a glob. The check fails when a value matches no file in the git index.

## Patterns

The `files` and `notowned` checks match the CODEOWNERS patterns with the rules of GitHub. GitHub uses most of the rules of [gitignore files](https://git-scm.com/docs/gitignore#_pattern_format):

- `*` matches any sequence of characters except `/`. `?` matches any one character except `/`.
- `**/` at the start of a pattern matches in all directories. `/**` at the end matches everything below a directory. `/**/` in the middle matches zero or more directories.
- A pattern with a `/` at the start or in the middle matches paths from the root of the repository. Another pattern matches at any level. For example, `apps/` matches the files in all the `apps` directories of the repository.
- A pattern that ends with `/` matches only directories.
- A pattern that matches a directory also matches all the files below the directory. For example, `/docs` matches `docs/guide/a.md`.

Paths are case-sensitive. The GitHub documentation gives these differences from gitignore files:

- `docs/*` matches `docs/getting-started.md`, but not `docs/build-app/troubleshooting.md`.
- Negation with `!`, character ranges with `[ ]`, and escaping a leading `#` with `\` do not work. The `syntax` check reports a warning for each pattern that starts with `!` or `\#`.

For the cases that the GitHub documentation does not describe, the command uses these rules:

- When the last segment of a pattern has `*` or `?`, the pattern does not match the files below a directory that the segment matches. For example, `docs/*.md` does not match `docs/a.md/b.txt`. This rule does not apply to a pattern that ends with `/`. For example, `docs/*/` matches `docs/guide/a.md`.
- `!`, `[`, `]`, and `\` match only themselves. For example, `/pages/[id].tsx` matches only the file `pages/[id].tsx`.
- In all other cases, the command uses the rules of gitignore files.

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

### Rate limit

The `owners` check uses the GitHub API rate limit of its credentials. Other clients of the same credentials, for example other tools in CI, use the same limit. To keep requests for them, the check does not validate the owners when fewer requests remain than `--github-rate-limit-reserve`. The default is 500. The value 0 disables the reserve. The command gets the number of remaining requests from each response. When fewer requests remain after `GET /repos/{owner}/{repo}`, the check does not validate the owners. When fewer requests remain after a later call, the check makes no more calls. It reports the issues that it found before, and that it is incomplete.

The check also does not validate the owners when `GET /repos/{owner}/{repo}` fails because of a rate limit. When a later call fails because of a rate limit, the check reports the issues that it found before the failure, and that it is incomplete.

In these cases, the output shows `[Skipped]` and the reason. A skipped check is not a failure: the command exits with 0, unless another check fails, or the skipped check found an issue at or above `--check-failure-level` before the skip.

### GitHub App

To authenticate as a GitHub App installation, set the App ID, the installation ID, and the private key of the App in PEM format:

| Flag                           | Environment variable         |
| ------------------------------ | ---------------------------- |
| `--github-app-id`              | `GITHUB_APP_ID`              |
| `--github-app-installation-id` | `GITHUB_APP_INSTALLATION_ID` |
| None                           | `GITHUB_APP_PRIVATE_KEY`     |

When the App ID is set, the command does not look for an access token. When GitHub refuses to create the installation token, for example because one of these values is wrong, the error names these settings.

### GitHub Enterprise Server

Set `--github-base-url` to the API base URL of the server, and `--github-upload-url` to its upload URL. The upload URL defaults to the base URL. The command does not get the server from the `origin` remote. Without `--github-base-url`, it uses the API of github.com.

`gh auth token` gives the token of the default host of `gh`, because the command does not give it `--hostname`. To use a token for the server, set `GITHUB_TOKEN` to the token, or set `GH_HOST` to the hostname of the server.

## Differences from codeowners-validator

- The `notowned` check does not change the repository. It lists the files in the git index, so it works with uncommitted changes and ignores untracked files. The upstream check reports an issue when `git status` shows changes. Otherwise, it appends the patterns to `.gitignore`, runs `git rm --cached` on the ignored files, and then runs `git reset --hard`.
- The `owners` check requires a user to be a collaborator of the repository with write access. The upstream check requires a user to be a member of the organization.
- The upstream command checks that a token has the `read:org` scope. This command does not check scopes, because fine-grained tokens and GitHub Apps use permissions instead of scopes.
- The command takes its settings from flags, except for some authentication settings (see [GitHub access](#github-access)). `--checks` selects both the default and the experimental checks. The upstream command uses environment variables, with `CHECKS` and `EXPERIMENTAL_CHECKS` for the checks.
- The `files` and `notowned` checks match the patterns with the rules of GitHub. See [Patterns](#patterns). The upstream `files` check matches glob patterns against the files in the working tree. The upstream `notowned` check uses the rules of gitignore files through `git ls-files`, so it matches without regard to case when the git setting `core.ignoreCase` is `true`. This command always matches case-sensitively.
- The `notowned` check reports a file when the last pattern that matches it has no owners. The upstream check reports a file only when no pattern matches it.
- `--trust-workspace` passes the repository directory as `safe.directory` to each git command. To find the root of the git work tree, it passes `*`, because the root is not known yet. The upstream command adds the directory to the global git configuration.
- When a check fails, the command exits with 1. The upstream command exits with 3.
- When the GitHub rate limit is low or reached, the `owners` check is skipped and does not make the command fail. The upstream command fails, or reports the rate limit as an issue.
