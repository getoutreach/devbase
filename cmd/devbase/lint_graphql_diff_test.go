// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Integration test for the "graphql" subcommand's --diff
// mode against a real git repository.

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
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
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	assert.NilError(t, err)

	commitFile(t, repo, root, "scripts/devbase.yaml", devbaseYAML)
	commitFile(t, repo, root, "schema.graphql", "type foo { a: String }\n")

	head, err := repo.Head()
	assert.NilError(t, err)
	baseName := head.Name().Short()

	wt, err := repo.Worktree()
	assert.NilError(t, err)
	assert.NilError(t, wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	}))
	commitFile(t, repo, root, "schema.graphql", "type foo { a: String }\ntype bar { b: String }\n")

	restoreCwd := chdir(t, root)
	defer restoreCwd()

	cmd := newLintGraphQLCommand()
	var out bytes.Buffer
	cmd.Writer = &out
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	runErr := cmd.Run(context.Background(), []string{"graphql", "--diff", "--base", baseName, "schema.graphql"})

	var exitErr cli.ExitCoder
	ok := errors.As(runErr, &exitErr)
	assert.Assert(t, ok, runErr)
	assert.Equal(t, exitErr.ExitCode(), 1)

	output := out.String()
	assert.Assert(t, strings.Contains(output, `"bar"`), output)
	assert.Assert(t, !strings.Contains(output, `"foo"`), output)
}

// chdir switches the working directory to dir and returns a function
// that restores the previous one.
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	prev, err := os.Getwd()
	assert.NilError(t, err)
	assert.NilError(t, os.Chdir(dir))
	return func() {
		assert.NilError(t, os.Chdir(prev))
	}
}
