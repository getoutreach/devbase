// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the lint codeowners subcommand.

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/gobox/pkg/cfg"
	"github.com/urfave/cli/v3"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

// codeownersDurationRE matches the duration that a check prints in its header.
var codeownersDurationRE = regexp.MustCompile(`\(\d[^)]*\)`)

// runGit runs git in dir and fails the test when git fails.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.NilError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// newCodeownersRepo creates a git repository in a temporary directory with the files, a CODEOWNERS
// file with the content in .github/, and everything staged. It returns the directory.
func newCodeownersRepo(t *testing.T, content string, files ...string) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init", "-q")

	for _, f := range files {
		assert.NilError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755))
		assert.NilError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600))
	}

	assert.NilError(t, os.MkdirAll(filepath.Join(dir, ".github"), 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".github", "CODEOWNERS"), []byte(content), 0o600))
	runGit(t, dir, "add", "-A")

	return dir
}

// runLintCodeowners runs the lint codeowners subcommand with args, and returns its output with
// the durations replaced, and the error that it returned.
func runLintCodeowners(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()

	out, _, err := runSubcommand(t, newLintCodeownersCommand(), "", args...)

	return codeownersDurationRE.ReplaceAllString(out, "(<duration>)"), err
}

// offlineChecks is the list of checks that need no network.
const offlineChecks = "files,duppatterns,syntax,avoid-shadowing,notowned"

// TestLintCodeowners_Success checks the output and the exit code when no check finds an issue.
func TestLintCodeowners_Success(t *testing.T) {
	repo := newCodeownersRepo(t, "* @org/team\n/main.go @org/team\n", "main.go", "docs/readme.md")

	out, err := runLintCodeowners(t, "--repository-path", repo, "--checks", offlineChecks)

	assert.NilError(t, err)
	golden.Assert(t, out, "lint_codeowners_success.golden")
}

// TestLintCodeowners_Failure checks the output and the exit code when checks find issues.
func TestLintCodeowners_Failure(t *testing.T) {
	repo := newCodeownersRepo(t,
		"/main.go @org/team\n/missing.go @org/team\n/main.go @org/other\n/lonely.go @-bad\n",
		"main.go", "docs/readme.md", "lonely.go")

	out, err := runLintCodeowners(t, "--repository-path", repo, "--checks", offlineChecks)

	assert.Equal(t, exitCode(t, err), 1)
	golden.Assert(t, out, "lint_codeowners_failure.golden")
}

// TestLintCodeowners_FailureLevel checks that --check-failure-level decides if warnings fail the run.
func TestLintCodeowners_FailureLevel(t *testing.T) {
	repo := newCodeownersRepo(t, "* @org/team\n/main.go @-bad\n", "main.go")

	out, err := runLintCodeowners(t, "--repository-path", repo, "--checks", "syntax")
	assert.Equal(t, exitCode(t, err), 1)
	assert.Assert(t, strings.Contains(out, "[war]"), out)

	out, err = runLintCodeowners(t, "--repository-path", repo, "--checks", "syntax", "--check-failure-level", "error")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(out, "[war]"), out)
}

// TestLintCodeowners_Staged checks that --staged reads CODEOWNERS and the files from the git
// index, and ignores the changes that are not staged.
func TestLintCodeowners_Staged(t *testing.T) {
	t.Run("notowned", func(t *testing.T) {
		repo := newCodeownersRepo(t, "/.github/ @org/team\n/main.go @org/team\n", "main.go", "x.txt")

		// An unstaged change of CODEOWNERS that owns x.txt.
		assert.NilError(t, os.WriteFile(filepath.Join(repo, ".github", "CODEOWNERS"),
			[]byte("/.github/ @org/team\n/main.go @org/team\n*.txt @org/team\n"), 0o600))

		out, err := runLintCodeowners(t, "--repository-path", repo, "--checks", "notowned")
		assert.NilError(t, err, out)

		out, err = runLintCodeowners(t, "--repository-path", repo, "--checks", "notowned", "--staged")
		assert.Equal(t, exitCode(t, err), 1)
		assert.Assert(t, strings.Contains(out, "Found 1 not owned files"), out)
		assert.Assert(t, strings.Contains(out, "* x.txt"), out)
	})

	t.Run("files", func(t *testing.T) {
		repo := newCodeownersRepo(t, "* @org/team\n/untracked.txt @org/team\n", "main.go")
		assert.NilError(t, os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x"), 0o600))

		out, err := runLintCodeowners(t, "--repository-path", repo, "--checks", "files")
		assert.NilError(t, err, out)

		out, err = runLintCodeowners(t, "--repository-path", repo, "--checks", "files", "--staged")
		assert.Equal(t, exitCode(t, err), 1)
		assert.Assert(t, strings.Contains(out, `"/untracked.txt" does not match any files in repository`), out)
	})
}

// TestLintCodeowners_RepositoryRoot checks that, without --repository-path, the command uses the
// root of the git work tree of the current directory, or the current directory outside of one.
// It changes the current directory, so it cannot run in parallel.
func TestLintCodeowners_RepositoryRoot(t *testing.T) {
	t.Run("from a subdirectory", func(t *testing.T) {
		repo := newCodeownersRepo(t, "* @org/team\n", "main.go", "sub/a.go")
		t.Chdir(filepath.Join(repo, "sub"))

		out, err := runLintCodeowners(t, "--checks", "files,notowned")
		assert.NilError(t, err, out)
	})

	t.Run("outside of a git work tree", func(t *testing.T) {
		dir := t.TempDir()
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "CODEOWNERS"), []byte("* @org/team\n"), 0o600))
		t.Chdir(dir)

		out, err := runLintCodeowners(t, "--checks", "syntax")
		assert.NilError(t, err, out)
	})
}

// TestRepositoryRoot checks the root of the repository without --repository-path. It changes the
// current directory and the environment, so it cannot run in parallel.
func TestRepositoryRoot(t *testing.T) {
	t.Run("not in a git work tree", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		root, err := repositoryRoot(t.Context(), "", false)
		assert.NilError(t, err)
		assert.Equal(t, root, dir)
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := repositoryRoot(ctx, "", false)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("git not found", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		_, err := repositoryRoot(t.Context(), "", false)
		assert.ErrorIs(t, err, exec.ErrNotFound)
	})
}

// TestRootCommand checks the arguments and the environment of the git command that finds the
// root of the work tree.
func TestRootCommand(t *testing.T) {
	t.Run("trusted", func(t *testing.T) {
		cmd := rootCommand(t.Context(), true)
		assert.DeepEqual(t, cmd.Args, []string{"git", "-c", "safe.directory=*", "rev-parse", "--show-toplevel"})
		assert.Assert(t, slices.Contains(cmd.Env, "LC_ALL=C"))
	})

	t.Run("not trusted", func(t *testing.T) {
		cmd := rootCommand(t.Context(), false)
		assert.DeepEqual(t, cmd.Args, []string{"git", "rev-parse", "--show-toplevel"})
		assert.Assert(t, slices.Contains(cmd.Env, "LC_ALL=C"))
	})
}

// TestLintCodeowners_Errors checks the errors that stop the command before any check runs.
func TestLintCodeowners_Errors(t *testing.T) {
	repo := newCodeownersRepo(t, "* @org/team\n", "main.go")

	t.Run("unknown check", func(t *testing.T) {
		_, err := runLintCodeowners(t, "--repository-path", repo, "--checks", "files,bogus")

		assert.ErrorContains(t, err, `unknown check "bogus"`)
		assert.ErrorContains(t, err, "avoid-shadowing, duppatterns, files, notowned, owners, syntax")
	})

	t.Run("unknown failure level", func(t *testing.T) {
		_, err := runLintCodeowners(t, "--repository-path", repo, "--check-failure-level", "fatal")

		assert.ErrorContains(t, err, `"fatal"`)
	})

	t.Run("no CODEOWNERS file", func(t *testing.T) {
		_, err := runLintCodeowners(t, "--repository-path", t.TempDir(), "--checks", "syntax")

		assert.ErrorContains(t, err, "No CODEOWNERS found")
	})
}

// TestLintCodeowners_CanceledContext checks that the command returns the error of the context.
func TestLintCodeowners_CanceledContext(t *testing.T) {
	repo := newCodeownersRepo(t, "* @org/team\n", "main.go")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cmd := newLintCodeownersCommand()
	var out bytes.Buffer
	cmd.Writer = &out
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	err := cmd.Run(ctx, []string{cmd.Name, "--repository-path", repo, "--checks", "syntax"})

	assert.ErrorIs(t, err, context.Canceled)
}

// TestParseCheckNames checks that check names keep their order and are not repeated.
func TestParseCheckNames(t *testing.T) {
	names, err := parseCheckNames([]string{"syntax", "files", "syntax", "owners", "files"})

	assert.NilError(t, err)
	assert.DeepEqual(t, names, []string{"syntax", "files", "owners"})

	_, err = parseCheckNames([]string{"syntax", "nope"})
	assert.ErrorIs(t, err, errUnknownCheck)
}

// TestRepositoryFromOrigin checks that the repository comes from the origin remote.
func TestRepositoryFromOrigin(t *testing.T) {
	for _, origin := range []string{
		"https://github.com/o/r.git",
		"https://github.com/o/r",
		"https://github.com/o/r/",
		"https://github.com/o/r.git/",
		"git@github.com:o/r.git",
		"ssh://git@github.com/o/r",
	} {
		t.Run(origin, func(t *testing.T) {
			dir := t.TempDir()
			runGit(t, dir, "init", "-q")
			runGit(t, dir, "remote", "add", "origin", origin)

			repo, err := repositoryFromOrigin(t.Context(), dir, false)

			assert.NilError(t, err)
			assert.Equal(t, repo, "o/r")
		})
	}

	t.Run("no origin", func(t *testing.T) {
		dir := t.TempDir()
		runGit(t, dir, "init", "-q")

		_, err := repositoryFromOrigin(t.Context(), dir, false)

		assert.ErrorContains(t, err, "origin")
	})

	t.Run("origin that is not a repository", func(t *testing.T) {
		dir := t.TempDir()
		runGit(t, dir, "init", "-q")
		runGit(t, dir, "remote", "add", "origin", "https://github.com/just-one")

		_, err := repositoryFromOrigin(t.Context(), dir, false)

		assert.ErrorIs(t, err, errInvalidOrigin)
		assert.ErrorContains(t, err, "https://github.com/just-one")
	})

	for _, origin := range []string{
		"https://user:secret@github.com/a/b/c.git",
		"https://secret@github.com/just-one",
		"https://user:secret%zz@github.com/o/r.git",
	} {
		t.Run("credentials are not shown for "+origin, func(t *testing.T) {
			dir := t.TempDir()
			runGit(t, dir, "init", "-q")
			runGit(t, dir, "remote", "add", "origin", origin)

			_, err := repositoryFromOrigin(t.Context(), dir, false)

			assert.ErrorIs(t, err, errInvalidOrigin)
			assert.ErrorContains(t, err, "github.com/")
			assert.Assert(t, !strings.Contains(err.Error(), "secret"), "got %v", err)
		})
	}
}

// TestRepositoryFromOriginParseError checks that the error for an origin URL that cannot be
// parsed has a fixed cause, and no part of the credentials in the URL.
func TestRepositoryFromOriginParseError(t *testing.T) {
	tests := map[string]string{
		"https://x-access-token:ghp_SECRETTOKEN#@github.com/o/r.git": "ghp_SECRETTOKEN",
		"https://user:s3cr3t%zz@github.com/o/r.git":                  "s3cr3t%zz",
		"https://user:s3cr3t?x@github.com/o/r.git":                   "s3cr3t?x",
		"https://user:s3cr3t#x@github.com/o/r.git":                   "s3cr3t#x",
	}
	for origin, password := range tests {
		t.Run(origin, func(t *testing.T) {
			dir := t.TempDir()
			runGit(t, dir, "init", "-q")
			runGit(t, dir, "remote", "add", "origin", origin)

			_, err := repositoryFromOrigin(t.Context(), dir, false)

			assert.ErrorIs(t, err, errInvalidOrigin)
			assert.Error(t, err,
				`the origin remote is not in the form owner/repository: "https://github.com/o/r.git": the URL cannot be parsed`)
			assert.Assert(t, !strings.Contains(err.Error(), password), "got %v", err)
		})
	}
}

// newFakeGitHubAPI starts a fake GitHub REST API for the repository o/r, where the user alice can
// push, and returns its URL. Its responses say that remaining requests of the rate limit remain.
func newFakeGitHubAPI(t *testing.T, remaining string) string {
	t.Helper()

	reply := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Ratelimit-Limit", "5000")
			w.Header().Set("X-Ratelimit-Remaining", remaining)
			_, _ = w.Write([]byte(body))
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v3/repos/o/r", reply(`{"name": "r"}`))
	mux.HandleFunc("GET /api/v3/repos/o/r/collaborators", reply(`[{"login": "alice", "permissions": {"push": true}}]`))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL
}

// fakeToken returns a GitHub token getter that counts its calls in calls.
func fakeToken(calls *int) tokenGetter {
	return func() (cfg.SecretData, error) {
		*calls++
		return "test-token", nil
	}
}

// runLintCodeownersWithToken runs the lint codeowners subcommand with args, with a GitHub token
// from getToken. It returns its output with the durations replaced, its standard error, and the
// error that it returned.
func runLintCodeownersWithToken(t *testing.T, getToken tokenGetter, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	out, errOut, err := runSubcommand(t, newLintCodeownersCommandWith(getToken), "", args...)

	return codeownersDurationRE.ReplaceAllString(out, "(<duration>)"), errOut, err
}

// TestLintCodeowners_RateLimitReserve checks that the owners check is skipped, and the command
// does not fail, when fewer GitHub API requests remain than --github-rate-limit-reserve.
func TestLintCodeowners_RateLimitReserve(t *testing.T) {
	repo := newCodeownersRepo(t, "* @alice\n", "main.go")
	args := []string{"--repository-path", repo, "--checks", "owners", "--repository", "o/r", "--github-base-url"}
	var calls int

	out, _, err := runLintCodeownersWithToken(t, fakeToken(&calls), append(args, newFakeGitHubAPI(t, "10"))...)
	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out,
		"    [Skipped] only 10 GitHub API requests remain before the rate limit resets, fewer than the reserve of 500\n"), out)
	assert.Assert(t, strings.Contains(out, "1 check(s) executed, no failure(s)"), out)

	out, _, err = runLintCodeownersWithToken(t, fakeToken(&calls),
		append(args, newFakeGitHubAPI(t, "10"), "--github-rate-limit-reserve", "10")...)
	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out, "    Check OK\n"), out)
}

// commitAll stages all the files in dir and commits them.
func commitAll(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "test")
}

// newBranchedCodeownersRepo creates a git repository with the branches main and feature, and
// feature checked out. The commit on main has .github/CODEOWNERS with the content base, or no
// CODEOWNERS file when base is empty. The next commit, on feature, changes the content to head.
func newBranchedCodeownersRepo(t *testing.T, base, head string) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o600))
	path := filepath.Join(dir, ".github", "CODEOWNERS")
	assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	if base != "" {
		assert.NilError(t, os.WriteFile(path, []byte(base), 0o600))
	}
	commitAll(t, dir)

	runGit(t, dir, "checkout", "-q", "-b", "feature")
	assert.NilError(t, os.WriteFile(path, []byte(head), 0o600))
	commitAll(t, dir)

	return dir
}

// ownersIfChangedArgs returns the arguments that run only the owners check with
// --owners-if-changed in repo, against a fake GitHub API where the user alice can push.
func ownersIfChangedArgs(t *testing.T, repo string) []string {
	t.Helper()

	return []string{
		"--repository-path", repo, "--checks", "owners", "--owners-if-changed",
		"--repository", "o/r", "--github-base-url", newFakeGitHubAPI(t, "5000"),
	}
}

// TestLintCodeowners_OwnersIfChanged checks that, with --owners-if-changed, the owners check runs
// only when CODEOWNERS changed since the merge base of HEAD and --base, and that it gets no GitHub
// token when it does not run.
func TestLintCodeowners_OwnersIfChanged(t *testing.T) {
	const owners = "* @alice\n"

	tests := map[string]struct {
		base, head string
		args       []string
		skipped    bool
		warning    string
	}{
		"unchanged":                 {base: owners, head: owners, skipped: true},
		"changed":                   {base: "* @bob\n", head: owners},
		"missing at the merge base": {base: "", head: owners},
		"unknown base":              {base: owners, head: owners, args: []string{"--base", "nope"}, warning: `"nope"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := newBranchedCodeownersRepo(t, tc.base, tc.head)
			var calls int

			out, stderr, err := runLintCodeownersWithToken(t, fakeToken(&calls), append(ownersIfChangedArgs(t, repo), tc.args...)...)

			assert.NilError(t, err, out)
			if tc.skipped {
				assert.Assert(t, strings.Contains(out,
					"    [Skipped] CODEOWNERS did not change since the merge base with main\n"), out)
				assert.Equal(t, calls, 0, "no GitHub token is needed")
			} else {
				assert.Assert(t, strings.Contains(out, "    Check OK\n"), out)
				assert.Equal(t, calls, 1)
			}
			if tc.warning == "" {
				assert.Equal(t, stderr, "")
			} else {
				assert.Assert(t, strings.HasPrefix(stderr, "warning: "), stderr)
				assert.Assert(t, strings.Contains(stderr, tc.warning), stderr)
				assert.Equal(t, strings.Count(stderr, "\n"), 1, stderr)
			}
		})
	}
}

// TestLintCodeowners_OwnersIfChangedShallowClone checks that the owners check runs, with a
// warning, in a shallow clone, which can be without the merge base.
func TestLintCodeowners_OwnersIfChangedShallowClone(t *testing.T) {
	src := newBranchedCodeownersRepo(t, "* @alice\n", "* @alice\n")
	repo := filepath.Join(t.TempDir(), "clone")
	runGit(t, src, "clone", "-q", "--depth", "1", "file://"+src, repo)
	var calls int

	out, stderr, err := runLintCodeownersWithToken(t, fakeToken(&calls), ownersIfChangedArgs(t, repo)...)

	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out, "    Check OK\n"), out)
	assert.Equal(t, calls, 1)
	assert.Equal(t, stderr,
		"warning: the owners check runs, because CODEOWNERS cannot be compared with the merge base of HEAD and main: "+
			"the repository is a shallow clone\n")
}

// TestLintCodeowners_OwnersIfChangedStaged checks that --owners-if-changed compares the CODEOWNERS
// file that the command loaded: the file in the working tree, or in the git index with --staged.
func TestLintCodeowners_OwnersIfChangedStaged(t *testing.T) {
	repo := newBranchedCodeownersRepo(t, "* @alice\n", "* @alice\n")
	assert.NilError(t, os.WriteFile(filepath.Join(repo, ".github", "CODEOWNERS"), []byte("# Not staged.\n* @alice\n"), 0o600))
	var calls int

	out, _, err := runLintCodeownersWithToken(t, fakeToken(&calls), ownersIfChangedArgs(t, repo)...)
	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out, "    Check OK\n"), out)

	out, _, err = runLintCodeownersWithToken(t, fakeToken(&calls), append(ownersIfChangedArgs(t, repo), "--staged")...)
	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out, "    [Skipped] CODEOWNERS did not change since the merge base with main\n"), out)
	assert.Equal(t, calls, 1, "only the run without --staged gets a token")
}

// TestLintCodeowners_OwnersIfChangedLinkedWorktree checks that --owners-if-changed reads the merge
// base in a linked git worktree, so that it skips the owners check when CODEOWNERS did not change.
func TestLintCodeowners_OwnersIfChangedLinkedWorktree(t *testing.T) {
	const owners = "* @alice\n"

	repo := newBranchedCodeownersRepo(t, owners, owners)
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo, "worktree", "add", "-q", "-b", "linked", worktree, "feature")
	var calls int

	out, stderr, err := runLintCodeownersWithToken(t, fakeToken(&calls), ownersIfChangedArgs(t, worktree)...)

	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out, "    [Skipped] CODEOWNERS did not change since the merge base with main\n"), out)
	assert.Equal(t, calls, 0, "no GitHub token is needed")
	assert.Equal(t, stderr, "")
}
