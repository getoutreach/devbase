// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the lint codeowners subcommand.

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
	})
}
