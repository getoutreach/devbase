// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Computes a release plan from the release model: the next
// version, the release notes and the assets, once, for the jobs after it.

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

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
	NewLinks func(repoURL string) NoteLinks

	// Now is the release date. Zero means the current time.
	Now time.Time
}

// MakePlan computes the release plan for a repository.
func MakePlan(ctx context.Context, r *Repo, opts *PlanOptions) (*Plan, error) {
	plan, repoURL, err := r.newPlan(ctx, opts.Branch)
	if err != nil {
		return nil, err
	}
	branchName := plan.Branch.Name
	branch := r.Config.Branch(branchName)
	if branch == nil {
		plan.Reason = fmt.Sprintf("branch %q is not a release branch", branchName)
		return plan, nil
	}
	plan.Branch = *branch

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
	plan.Notes = RenderNotes(&NotesInput{
		Version: next.String(), Tag: plan.NextRelease.GitTag, PreviousTag: from,
		Date: now, Commits: commits, Links: opts.NewLinks(repoURL),
	})
	if a := r.Config.Artifacts; a != nil {
		for _, target := range a.Targets {
			plan.Targets = append(plan.Targets, target.String())
			plan.Assets = append(plan.Assets, ArchiveName(plan.ProjectName, next.String(), &target))
		}
		plan.Assets = append(plan.Assets, a.Checksums)
	}
	return plan, nil
}

// newPlan returns a plan with the HEAD commit, the project name and the
// branch filled in, and the repository URL.
func (r *Repo) newPlan(ctx context.Context, branch string) (*Plan, string, error) {
	out, err := r.Git.Run(ctx, "log", "-1", "--format=%H %ct", "HEAD")
	if err != nil {
		return nil, "", err
	}
	head, ct, _ := strings.Cut(out, " ")
	commitTime, err := strconv.ParseInt(ct, 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("parse commit time %q: %w", ct, err)
	}
	repoURL, err := r.Git.RepoURL(ctx)
	if err != nil {
		return nil, "", err
	}
	if branch == "" {
		if branch, err = r.Git.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD"); err != nil {
			return nil, "", err
		}
	}
	return &Plan{
		Schema: PlanSchema, Commit: head, CommitTime: commitTime, ProjectName: path.Base(repoURL),
		ConfigDigest: r.Digest, Branch: spec.Branch{Name: branch},
	}, repoURL, nil
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
