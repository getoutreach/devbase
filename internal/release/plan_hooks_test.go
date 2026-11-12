// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for plan --run-hooks, plan --dry-run and the debug
// log.

package release

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getoutreach/devbase/v2/internal/release/forge/github"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"gotest.tools/v3/assert"
)

// hooksConfig has a generateNotesCmd hook before and after the notes
// plugin.
const hooksConfig = `preset: conventionalcommits
branches: [master]
plugins:
  - "@semantic-release/commit-analyzer"
  - - "@semantic-release/exec"
    - generateNotesCmd: "echo before ${nextRelease.version} ${options.dryRun}; echo to stderr >&2"
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/exec"
    - generateNotesCmd: "printf '  after %s \\n' \"$SOME_TOKEN\""
`

// planHooks plans the release of a repository with hooksConfig. A
// non-nil debug gets the debug log.
func planHooks(t *testing.T, debug io.Writer, opts *PlanOptions) (plan *Plan, hookLog string) {
	t.Helper()
	// Fixed dates give the same commit hashes in each repository.
	t.Setenv("GIT_AUTHOR_DATE", "2026-11-11T12:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-11-11T12:00:00Z")
	f := newRepo(t, "master")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	writeFile(t, f.git.Dir, ".releaserc.yaml", hooksConfig)
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	repo, err := LoadRepo(f.git.Dir)
	assert.NilError(t, err)
	var log bytes.Buffer
	opts.Now, opts.NewLinks, opts.Log = time.Unix(0, 0), github.NewLinks, &log
	opts.Env = append(os.Environ(), "SOME_TOKEN=supersecret")
	ctx := context.Background()
	if debug != nil {
		ctx = rlog.WithDebug(ctx, debug)
	}
	plan, err = MakePlan(ctx, repo, opts)
	assert.NilError(t, err)
	return plan, log.String()
}

func TestPlanRunHooks(t *testing.T) {
	// Without --run-hooks, the notes are only the notes plugin's.
	plain, log := planHooks(t, nil, &PlanOptions{})
	assert.Equal(t, log, "")
	assert.Assert(t, strings.HasPrefix(plain.Notes, "## 1.0.0"), plain.Notes)

	// With it, the hook outputs join the notes in plugin order, trimmed,
	// with the secrets masked, and the hooks' stderr goes to the log.
	plan, log := planHooks(t, nil, &PlanOptions{RunHooks: true})
	assert.Equal(t, plan.Notes, "before 1.0.0 false\n\n"+plain.Notes+"\n\nafter [secure]")
	assert.Assert(t, strings.Contains(log, "to stderr\n"), log)
	assert.Assert(t, !plan.DryRun)
}

func TestPlanDryRun(t *testing.T) {
	plan, _ := planHooks(t, nil, &PlanOptions{RunHooks: true, DryRun: true})
	assert.Assert(t, plan.DryRun)
	assert.Assert(t, strings.HasPrefix(plan.Notes, "before 1.0.0 true\n\n"), plan.Notes)
}

func TestPlanDebugLog(t *testing.T) {
	var debug bytes.Buffer
	planHooks(t, &debug, &PlanOptions{})
	assert.Assert(t, strings.Contains(debug.String(), "debug: run git log -1 --format=%H %ct HEAD\n"), debug.String())
}

func TestPublishRefusesDryRunPlan(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	plan.DryRun = true
	assert.ErrorIs(t, p.publish(repo, plan), ErrDryRunPlan)
	assert.Equal(t, len(p.forge.Calls), 0)

	// publish --dry-run checks a dry-run plan.
	assert.NilError(t, Publish(context.Background(), repo, plan, &PublishOptions{
		Dist: "dist", Remote: p.remote, Forge: p.forge, Env: os.Environ(), DryRun: true,
	}))
}
