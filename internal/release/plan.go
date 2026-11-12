// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Computes a release plan from the release model: the next
// version, the release notes and the assets, once, for the jobs after it.

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/internal/js"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// PlanSchema is the version of the plan file format.
const PlanSchema = 1

// These are the errors that ReadPlan returns.
var (
	// ErrPlanSchema is returned for a plan file with another schema.
	ErrPlanSchema = errors.New("unsupported plan file schema")

	// ErrPlanConfig is returned when the configuration changed after
	// the plan was made.
	ErrPlanConfig = errors.New("release configuration changed after planning")
)

// Plan is the result of planning a release. It is written to a file so
// that build and publish jobs use the same version.
type Plan struct {
	Schema  int    `json:"schema"`
	Release bool   `json:"release"`
	Reason  string `json:"reason,omitempty"`

	// DryRun is true for a plan from plan --dry-run, which publish
	// refuses.
	DryRun bool `json:"dryRun,omitempty"`

	// Commit is the planned commit, and CommitTime its committer time
	// in Unix seconds, which archives use as file times.
	Commit     string `json:"commit"`
	CommitTime int64  `json:"commitTime"`

	// ProjectName is the archive name prefix: the repository name.
	ProjectName string `json:"projectName"`

	Branch      spec.Branch  `json:"branch"`
	LastRelease *PlanRelease `json:"lastRelease,omitempty"`
	NextRelease *PlanRelease `json:"nextRelease,omitempty"`
	Notes       string       `json:"notes,omitempty"`

	// Targets are the build targets as "os/arch".
	Targets []string `json:"targets,omitempty"`

	// Assets are the files that the release uploads.
	Assets []string `json:"assets,omitempty"`

	// ConfigDigest is Repo.Digest when the plan was made.
	ConfigDigest string `json:"configDigest"`
}

// PlanRelease describes one release in a plan.
type PlanRelease struct {
	Type    spec.Bump `json:"type,omitempty"`
	Version string    `json:"version"`
	GitTag  string    `json:"gitTag"`
	Channel string    `json:"channel,omitempty"`
}

// PlanOptions are the inputs of MakePlan.
type PlanOptions struct {
	// Branch is the branch to plan for. Empty means the current branch.
	Branch string

	// NewLinks returns the links for the release notes of the
	// repository at repoURL, so that the plan does not depend on one forge.
	NewLinks func(repoURL string) forge.NoteLinks

	// Now is the release date. Zero means the current time.
	Now time.Time

	// Remote is the URL to fetch tags, the branch and notes from, and to
	// compare the branch with. Empty means that the plan uses only local
	// refs. A forge can give a URL with credentials.
	Remote string

	// DryRun marks the plan as a dry run, which publish refuses. The
	// hooks see options.dryRun as true.
	DryRun bool

	// RunHooks runs the generateNotesCmd hooks, whose stdout joins the
	// notes, as in semantic-release.
	RunHooks bool

	// Env is the environment of the hooks. Its secrets are masked in the
	// notes.
	Env []string

	// Log receives the stderr of the hooks.
	Log io.Writer
}

// MakePlan computes the release plan for a repository.
func MakePlan(ctx context.Context, r *Repo, opts *PlanOptions) (_ *Plan, err error) {
	ctx, end := startStep(ctx, "plan")
	defer end(&err)
	plan, repoURL, current, err := r.newPlan(ctx, opts.Branch)
	if err != nil {
		return nil, err
	}
	plan.DryRun = opts.DryRun
	branchName := plan.Branch.Name
	branch := r.Config.Branch(branchName)
	if branch == nil {
		plan.Reason = fmt.Sprintf("branch %q is not a release branch", branchName)
		return plan, nil
	}
	plan.Branch = *branch

	if opts.Remote != "" {
		canPush, err := r.syncRemote(ctx, opts.Remote, branchName, current, plan.Commit)
		if err != nil {
			return nil, err
		}
		if !canPush {
			plan.Reason = fmt.Sprintf("the local branch %q is behind the remote one", branchName)
			return plan, nil
		}
	}

	tags, err := r.Git.Tags(ctx, "HEAD")
	if err != nil {
		return nil, err
	}
	last := LastRelease(branch, tags)
	from := ""
	if last != nil {
		plan.LastRelease = &PlanRelease{Version: last.Version.String(), GitTag: last.Name}
		from = last.Name
	}

	raws, err := r.Git.Commits(ctx, from, "HEAD")
	if err != nil {
		return nil, err
	}
	commits := ParseCommits(raws)
	t := Analyze(DropSkipped(commits), r.Config.Rules)
	if t == spec.BumpNone {
		plan.Reason = "no relevant changes"
		return plan, nil
	}

	next := NextVersion(branch, last, tags, t)
	if r.Config.MajorCheck {
		if err := CheckMajor(last, next); err != nil {
			return nil, err
		}
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	plan.Release = true
	plan.NextRelease = &PlanRelease{Type: t, Version: next.String(), GitTag: "v" + next.String(), Channel: branch.Channel}
	notes := RenderNotes(&NotesInput{
		Version: next.String(), Tag: plan.NextRelease.GitTag, PreviousTag: from,
		Date: now, Commits: commits, Links: opts.NewLinks(repoURL),
	})
	if plan.Notes, err = r.generateNotes(ctx, plan, notes, opts); err != nil {
		return nil, err
	}
	plan.addArtifacts(r.Config.Artifacts)
	return plan, nil
}

// addArtifacts adds the build targets and the release assets of a to the
// plan of a release, or nothing when a is nil.
func (p *Plan) addArtifacts(a *spec.Artifacts) {
	if a == nil {
		return
	}
	p.Targets = planTargets(a)
	for _, target := range a.Targets {
		p.Assets = append(p.Assets, ArchiveName(p.ProjectName, p.NextRelease.Version, &target))
	}
	p.Assets = append(p.Assets, a.Checksums)
}

// generateNotes runs the generateNotes step, as semantic-release does:
// in plugin order, the notes plugin gives notes, and with
// opts.RunHooks, each exec generateNotesCmd hook gives its stdout. The
// outputs that are not empty are joined with a blank line, and the
// secrets are masked with the Masker of ctx, or else of opts.Env.
func (r *Repo) generateNotes(ctx context.Context, plan *Plan, notes string, opts *PlanOptions) (string, error) {
	var parts []string
	for _, pl := range r.Config.Plugins {
		switch {
		case pl.Notes != nil:
			parts = append(parts, notes)
		case opts.RunHooks && pl.Exec != nil && pl.Exec.GenerateNotes != "":
			out, err := runExecHook(ctx, r.Git.Dir, opts.Env, logOr(opts.Log), pl.Exec.GenerateNotes, plan.execFields())
			if err != nil {
				return "", fmt.Errorf("generateNotes hook: %w", err)
			}
			parts = append(parts, out)
		}
	}
	parts = slices.DeleteFunc(parts, func(s string) bool { return s == "" })
	m := rlog.MaskerFrom(ctx)
	if m == nil {
		m = rlog.NewMasker(opts.Env)
	}
	return m.Mask(strings.Join(parts, "\n\n")), nil
}

// execFields returns the values of the ${...} lookups of exec commands
// for the release of the plan.
func (p *Plan) execFields() *spec.ExecFields {
	f := &spec.ExecFields{
		NextVersion: p.NextRelease.Version, NextTag: p.NextRelease.GitTag,
		NextChannel: p.NextRelease.Channel, Branch: p.Branch.Name, DryRun: p.DryRun,
	}
	if last := p.LastRelease; last != nil {
		f.LastVersion, f.LastTag = last.Version, last.GitTag
	}
	return f
}

// runExecHook runs an exec hook with /bin/sh in dir, as @semantic-release/exec
// does, and returns its stdout without the surrounding whitespace. The
// rendered command and the hook's stderr go to log.
func runExecHook(ctx context.Context, dir string, env []string, log io.Writer, cmd string, fields *spec.ExecFields) (string, error) {
	rendered := spec.RenderExecCommand(cmd, fields)
	fmt.Fprintf(log, "Run %s\n", rendered)
	c := exec.CommandContext(ctx, "/bin/sh", "-c", rendered)
	c.Dir, c.Env, c.Stderr = dir, env, log
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", rendered, err)
	}
	return js.Trim(string(out)), nil
}

// planTargets returns the build targets of a, as os/arch strings.
func planTargets(a *spec.Artifacts) []string {
	targets := make([]string, 0, len(a.Targets))
	for _, t := range a.Targets {
		targets = append(targets, t.String())
	}
	return targets
}

// syncRemote fetches the tags, the branch and the notes from the remote
// at url, as semantic-release does before it plans, and reports whether
// a push of HEAD to the branch would be a fast-forward. semantic-release
// checks that with git push --dry-run, which needs credentials, and does
// not release when the push fails because the local branch is behind.
// The plan runs without credentials, so it checks the same condition
// with the fetched refs.
func (r *Repo) syncRemote(ctx context.Context, url, branch, current, head string) (bool, error) {
	if err := r.Git.Fetch(ctx, url, branch, current); err != nil {
		return false, fmt.Errorf("fetch %s: %w", branch, err)
	}
	r.Git.FetchNotes(ctx, url)
	remote, err := r.Git.RemoteHead(ctx, url, branch)
	if err != nil {
		return false, err
	}
	return remote == "" || remote == head || r.Git.IsAncestor(ctx, remote, head), nil
}

// newPlan returns a plan with the HEAD commit, the project name and the
// branch filled in, the repository URL, and the checked-out branch, which
// is "HEAD" when HEAD is detached.
func (r *Repo) newPlan(ctx context.Context, branch string) (plan *Plan, repoURL, current string, err error) {
	out, err := r.Git.Run(ctx, "log", "-1", "--format=%H %ct", "HEAD")
	if err != nil {
		return nil, "", "", err
	}
	head, ct, _ := strings.Cut(out, " ")
	commitTime, err := strconv.ParseInt(ct, 10, 64)
	if err != nil {
		return nil, "", "", fmt.Errorf("parse commit time %q: %w", ct, err)
	}
	if repoURL, err = r.Git.RepoURL(ctx); err != nil {
		return nil, "", "", err
	}
	if current, err = r.Git.CurrentBranch(ctx); err != nil {
		return nil, "", "", err
	}
	if branch == "" {
		branch = current
	}
	return &Plan{
		Schema: PlanSchema, Commit: head, CommitTime: commitTime, ProjectName: path.Base(repoURL),
		ConfigDigest: r.Digest, Branch: spec.Branch{Name: branch},
	}, repoURL, current, nil
}

// WritePlan writes a plan as JSON.
func WritePlan(file string, p *Plan) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plan: %w", err)
	}
	if err := os.WriteFile(file, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}

// ReadPlan reads a plan and checks that it matches the schema and the
// configuration of r.
func ReadPlan(file string, r *Repo) (*Plan, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("decode plan: %w", err)
	}
	if p.Schema != PlanSchema {
		return nil, fmt.Errorf("%w: %d", ErrPlanSchema, p.Schema)
	}
	if p.ConfigDigest != r.Digest {
		return nil, ErrPlanConfig
	}
	return &p, nil
}
