// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: End-to-end tests for MakePlan with a git repository.

package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// fixtureConfig is a stencil-style config with an rc prerelease branch.
const fixtureConfig = `preset: conventionalcommits
branches:
  - name: main
    prerelease: rc
  - name: release
plugins:
  - - "@semantic-release/commit-analyzer"
    - releaseRules:
        - breaking: true
          release: minor
        - type: revert
          release: patch
        - type: perf
          release: patch
  - "@semantic-release/release-notes-generator"
`

// fixture is a git repository for plan tests.
type fixture struct {
	t   *testing.T
	git *Git
}

// newFixture creates a repository on branch main with fixtureConfig.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := &fixture{t: t, git: &Git{Dir: t.TempDir()}}
	f.run("init", "-q", "-b", "main")
	f.run("config", "user.email", "test@example.com")
	f.run("config", "user.name", "Test")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	assert.NilError(t, os.WriteFile(filepath.Join(f.git.Dir, ".releaserc.yaml"), []byte(fixtureConfig), 0o600))
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	return f
}

// run runs git in the fixture and fails the test on error.
func (f *fixture) run(args ...string) string {
	f.t.Helper()
	out, err := f.git.Run(context.Background(), args...)
	assert.NilError(f.t, err)
	return out
}

// commit makes an empty commit with message msg.
func (f *fixture) commit(msg string) {
	f.t.Helper()
	f.run("commit", "-q", "--allow-empty", "-m", msg)
}

// release plans a release on the current branch, checks the version,
// and tags it as semantic-release would.
func (f *fixture) release(branch, want string) {
	f.t.Helper()
	f.run("checkout", "-q", branch)
	plan, err := f.plan()
	assert.NilError(f.t, err)
	if want == "" {
		assert.Assert(f.t, !plan.Release, "unexpected release %+v", plan.NextRelease)
		return
	}
	assert.Assert(f.t, plan.Release, plan.Reason)
	assert.Equal(f.t, plan.NextRelease.Version, want)
	f.addTag(plan.NextRelease.GitTag, plan.NextRelease.Channel)
}

// plan loads the fixture repository and plans a release.
func (f *fixture) plan() (*Plan, error) {
	f.t.Helper()
	repo, err := LoadRepo(f.git.Dir)
	assert.NilError(f.t, err)
	return MakePlan(context.Background(), repo, &PlanOptions{
		Now:      time.Unix(0, 0),
		NewLinks: func(repoURL string) NoteLinks { return GitHubLinks{RepoURL: repoURL} },
	})
}

// addTag tags HEAD and adds the git note with its channel, as
// semantic-release does.
func (f *fixture) addTag(tag, channel string) {
	f.t.Helper()
	note := `{"channels":[null]}`
	if channel != "" {
		note = `{"channels":["` + channel + `"]}`
	}
	f.run("tag", tag)
	f.run("notes", "--ref", "semantic-release-"+tag, "add", "-f", "-m", note, tag)
}

// TestMakePlanPrereleaseFlow replays the flow that was checked against
// semantic-release 25.0.9 with a local remote.
func TestMakePlanPrereleaseFlow(t *testing.T) {
	f := newFixture(t)
	f.run("branch", "release")

	f.release("release", "1.0.0")

	f.run("checkout", "-q", "main")
	f.commit("feat: a")
	f.release("main", "1.1.0-rc.1")

	f.commit("fix: b")
	f.release("main", "1.1.0-rc.2")

	f.commit("feat!: c") // breaking -> minor with the custom rules
	f.release("main", "1.1.0-rc.3")

	f.run("checkout", "-q", "release")
	f.run("merge", "-q", "--ff-only", "main")
	f.release("release", "1.1.0")

	f.run("checkout", "-q", "main")
	f.commit("fix: d")
	f.release("main", "1.1.1-rc.1")

	f.commit("chore: e")
	f.release("main", "")

	note := f.run("notes", "--ref", "semantic-release-v1.1.1-rc.1", "show", "v1.1.1-rc.1")
	assert.Equal(t, note, `{"channels":["main"]}`)
	note = f.run("notes", "--ref", "semantic-release-v1.1.0", "show", "v1.1.0")
	assert.Equal(t, note, `{"channels":[null]}`)
}

func TestMakePlanUnknownBranch(t *testing.T) {
	f := newFixture(t)
	f.run("checkout", "-q", "-b", "feature")
	plan, err := f.plan()
	assert.NilError(t, err)
	assert.Assert(t, !plan.Release)
	assert.Assert(t, strings.Contains(plan.Reason, "not a release branch"), plan.Reason)
}

func TestReadPlanChecksConfig(t *testing.T) {
	f := newFixture(t)
	f.run("branch", "release")
	f.run("checkout", "-q", "release")
	plan, err := f.plan()
	assert.NilError(t, err)
	file := filepath.Join(t.TempDir(), "plan.json")
	assert.NilError(t, WritePlan(file, plan))

	repo, err := LoadRepo(f.git.Dir)
	assert.NilError(t, err)
	got, err := ReadPlan(file, repo)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, plan)

	cfg := fixtureConfig + "  - \"@semantic-release/github\"\n"
	assert.NilError(t, os.WriteFile(filepath.Join(f.git.Dir, ReleaseConfigFile), []byte(cfg), 0o600))
	repo, err = LoadRepo(f.git.Dir)
	assert.NilError(t, err)
	_, err = ReadPlan(file, repo)
	assert.ErrorIs(t, err, ErrPlanConfig)
}

func TestMakePlanMajorCheck(t *testing.T) {
	f := newFixture(t)
	cfg := strings.Replace(fixtureConfig, "        - breaking: true\n          release: minor\n", "", 1) +
		`  - - "@semantic-release/exec"
    - generateNotesCmd: ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
`
	assert.NilError(t, os.WriteFile(filepath.Join(f.git.Dir, ".releaserc.yaml"), []byte(cfg), 0o600))
	f.run("commit", "-q", "-am", "chore: config")
	f.run("branch", "release")
	f.release("release", "1.0.0")

	f.run("checkout", "-q", "release")
	f.commit("feat!: breaking")
	_, err := f.plan()
	assert.ErrorIs(t, err, ErrMajorVersion)
}
