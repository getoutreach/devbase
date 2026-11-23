// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Integration test for the "graphql" subcommand's --diff
// mode against a real git repository.

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/urfave/cli/v3"
	"gotest.tools/v3/assert"
)

// devbaseYAML enables naming-convention (a Tier 3 rule, off by
// default) so the fixture below can carry two simultaneous
// violations: unlike a Tier 1 rule, gqlparser doesn't stop at the
// first one it finds.
const devbaseYAML = `
graphql:
  lint:
    rules:
      naming-convention:
        - error
        - types: PascalCase
`

// commitFile writes path (relative to root) with content, stages it,
// and commits it to repo's current branch.
func commitFile(t *testing.T, repo *git.Repository, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	assert.NilError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	assert.NilError(t, os.WriteFile(full, []byte(content), 0o600))

	wt, err := repo.Worktree()
	assert.NilError(t, err)
	_, err = wt.Add(path)
	assert.NilError(t, err)
	sig := &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(0, 0)}
	_, err = wt.Commit("commit "+path, &git.CommitOptions{Author: sig})
	assert.NilError(t, err)
}

// TestLintGraphQLDiff builds a repository whose base branch already
// has one naming-convention violation ("foo"), then checks out a
// feature branch that adds a second ("bar") without fixing the first.
// It runs `devbase graphql lint --diff --base <base>` from the
// repository root and confirms only the "bar" violation -- the one
// that is new since the merge-base -- is reported, and that it fails
// the run.
func TestLintGraphQLDiff(t *testing.T) {
	root, baseName := newDiffRepo(t)

	out, runErr := runGraphQLDiff(t, root, baseName)

	assertOnlyNewViolation(t, out, runErr)
}

// TestLintGraphQLDiffBloblessClone runs the same check in a blobless clone, where the merge-base
// version of the schema file is not in the clone. The result must be the same as in a full clone:
// only the new "bar" violation, and not the old "foo" violation.
func TestLintGraphQLDiffBloblessClone(t *testing.T) {
	root, baseName := newDiffRepo(t)
	clone := newDiffPartialClone(t, root, "blob:none")

	out, runErr := runGraphQLDiff(t, clone, "origin/"+baseName)

	assertOnlyNewViolation(t, out, runErr)
}

// TestLintGraphQLDiffTreelessClone runs the check in a clone without trees. It cannot read the
// merge-base version of the schema file, so the run must fail with the advice for CircleCI, and must
// not report wrong violations.
func TestLintGraphQLDiffTreelessClone(t *testing.T) {
	root, baseName := newDiffRepo(t)
	clone := newDiffPartialClone(t, root, "tree:0")

	_, err := runGraphQLDiff(t, clone, "origin/"+baseName)
	assert.ErrorContains(t, err, "set `checkout: method: full`")
}

// newDiffPartialClone clones the repository at root without the objects that filter excludes, and
// checks out its feature branch.
func newDiffPartialClone(t *testing.T, root, filter string) string {
	t.Helper()

	runGit(t, root, "config", "uploadpack.allowFilter", "true")

	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(clone), "clone", "-q", "--filter="+filter, "--no-checkout", "file://"+root, clone)
	runGit(t, clone, "checkout", "-q", "feature")

	return clone
}

// assertOnlyNewViolation checks that the run failed because of the new "bar" violation, and that the
// old "foo" violation of the base branch is not reported.
func assertOnlyNewViolation(t *testing.T, out string, runErr error) {
	t.Helper()

	var exitErr cli.ExitCoder
	ok := errors.As(runErr, &exitErr)
	assert.Assert(t, ok, runErr)
	assert.Equal(t, exitErr.ExitCode(), 1)

	assert.Assert(t, strings.Contains(out, `"bar"`), out)
	assert.Assert(t, !strings.Contains(out, `"foo"`), out)
}

// runGraphQLDiff runs `graphql --diff --base base schema.graphql` from dir and returns the output
// and the error of the command.
func runGraphQLDiff(t *testing.T, dir, base string) (string, error) {
	t.Helper()

	t.Chdir(dir)

	cmd := newLintGraphQLCommand()
	var out bytes.Buffer
	cmd.Writer = &out
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	err := cmd.Run(t.Context(), []string{"graphql", "--diff", "--base", base, "schema.graphql"})

	return out.String(), err
}

// runGit runs git in dir, without the git configuration of the user or the system. It fails the
// test when git fails.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	assert.NilError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// newDiffRepo builds a repository whose base branch already has one naming-convention violation
// ("foo"), then checks out a feature branch that adds a second ("bar") without fixing the first.
// It returns the repository root and the name of the base branch.
func newDiffRepo(t *testing.T) (root, baseName string) {
	t.Helper()

	root = t.TempDir()
	repo, err := git.PlainInit(root, false)
	assert.NilError(t, err)

	commitFile(t, repo, root, "scripts/devbase.yaml", devbaseYAML)
	commitFile(t, repo, root, "schema.graphql", "type foo { a: String }\n")

	head, err := repo.Head()
	assert.NilError(t, err)
	baseName = head.Name().Short()

	wt, err := repo.Worktree()
	assert.NilError(t, err)
	assert.NilError(t, wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	}))
	commitFile(t, repo, root, "schema.graphql", "type foo { a: String }\ntype bar { b: String }\n")

	return root, baseName
}
