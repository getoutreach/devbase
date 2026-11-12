// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Plans the release units: which commits belong to each
// unit and to the root release, and the next version of each unit.

package release

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the errors of the unit plan.
var (
	// ErrUnitNoTag is returned for an independent unit that has no tag
	// yet. The baseline command tags its current version.
	ErrUnitNoTag = errors.New("the release unit has no tag")

	// ErrUnitVersion is returned when a mirror unit would get a version
	// that is not higher than all its tags.
	ErrUnitVersion = errors.New("the release unit version is not new")

	// ErrUnknownUnit is returned for a unit name that the config does
	// not have.
	ErrUnknownUnit = errors.New("unknown release unit")
)

// placeholderVersions are the package.json versions of a package whose
// version only the release sets.
//
//nolint:gochecknoglobals // Why: read-only list.
var placeholderVersions = []string{"", "0.0.0-development", "0.0.0-semantically-released"}

// PlanUnit is the release of one unit in a plan.
type PlanUnit struct {
	Name     string            `json:"name"`
	Kind     spec.UnitKind     `json:"kind"`
	Dir      string            `json:"dir"`
	Strategy spec.UnitStrategy `json:"strategy"`
	Release  bool              `json:"release"`
	Reason   string            `json:"reason,omitempty"`

	LastRelease *PlanRelease `json:"lastRelease,omitempty"`
	NextRelease *PlanRelease `json:"nextRelease,omitempty"`
	Notes       string       `json:"notes,omitempty"`

	// Commits are the hashes of the commits of the release, newest
	// first, for the success step.
	Commits []string `json:"commits,omitempty"`
}

// RootRelease reports whether the root releases. Release is also true
// when only a unit releases.
func (p *Plan) RootRelease() bool {
	return p.NextRelease != nil
}

// ReleasedUnits returns the units of the plan that release.
func (p *Plan) ReleasedUnits() []*PlanUnit {
	var out []*PlanUnit
	for i := range p.Units {
		if p.Units[i].Release {
			out = append(out, &p.Units[i])
		}
	}
	return out
}

// attribution gives the commits to the units and the root release by
// the files that they change.
type attribution struct {
	units []spec.Unit
	root  spec.RootPaths
}

// owns reports whether file belongs to u: it matches the paths of u, no
// exclude of u, and is not in the folder of a unit nested in u.
func (a *attribution) owns(u *spec.Unit, file string) bool {
	if !spec.MatchAnyPath(u.Paths, file) || spec.MatchAnyPath(u.Exclude, file) {
		return false
	}
	for i := range a.units {
		v := &a.units[i]
		if v != u && strings.HasPrefix(v.Dir, u.Dir+"/") && strings.HasPrefix(file, v.Dir+"/") {
			return false
		}
	}
	return true
}

// unitGets reports whether a commit with files belongs to u.
func (a *attribution) unitGets(u *spec.Unit, files []string) bool {
	return slices.ContainsFunc(files, func(f string) bool { return a.owns(u, f) })
}

// rootGets reports whether a commit with files belongs to the root
// release. The root files are the files that no root exclude matches.
// The commit belongs to the root when a root file is not shared, or when
// it has root files and belongs to no unit. A commit without files, such
// as the empty commit of the release candidate flow or a merge commit,
// belongs to the root.
func (a *attribution) rootGets(files []string) bool {
	if a.root.IsZero() || len(files) == 0 {
		return true
	}
	rootFiles := false
	for _, f := range files {
		if spec.MatchAnyPath(a.root.Exclude, f) {
			continue
		}
		if !spec.MatchAnyPath(a.root.Shared, f) {
			return true
		}
		rootFiles = true
	}
	if !rootFiles {
		return false
	}
	for i := range a.units {
		if a.unitGets(&a.units[i], files) {
			return false
		}
	}
	return true
}

// filterCommits returns the commits that keep reports true for, without
// their files.
func filterCommits(commits []FileCommit, keep func(files []string) bool) []RawCommit {
	var out []RawCommit
	for _, c := range commits {
		if keep(c.Files) {
			out = append(out, c.RawCommit)
		}
	}
	return out
}

// rootCommits returns the commits of the root release in from..to:
// every commit, or with root path rules, the commits that belong to the
// root.
func (r *Repo) rootCommits(ctx context.Context, from, to string) ([]RawCommit, error) {
	if r.Config.RootPaths.IsZero() {
		return r.Git.Commits(ctx, from, to)
	}
	commits, err := r.Git.CommitsWithFiles(ctx, from, to)
	if err != nil {
		return nil, err
	}
	a := &attribution{units: r.Config.Units, root: r.Config.RootPaths}
	return filterCommits(commits, a.rootGets), nil
}

// unitPlanner plans the units of one plan.
type unitPlanner struct {
	r      *Repo
	plan   *Plan
	branch *spec.Branch
	links  forge.NoteLinks
	now    time.Time
	opts   *PlanOptions
}

// planUnits adds the units of the config, or the ones that opts.Units
// names, to the plan. The plan releases when a unit releases.
func (r *Repo) planUnits(
	ctx context.Context, plan *Plan, branch *spec.Branch, links forge.NoteLinks, now time.Time, opts *PlanOptions,
) error {
	for _, name := range opts.Units {
		if r.Config.Unit(name) == nil {
			return fmt.Errorf("%w: %q", ErrUnknownUnit, name)
		}
	}
	p := &unitPlanner{r: r, plan: plan, branch: branch, links: links, now: now, opts: opts}
	for i := range r.Config.Units {
		u := &r.Config.Units[i]
		if len(opts.Units) > 0 && !slices.Contains(opts.Units, u.Name) {
			continue
		}
		pu, err := p.plan1(ctx, u)
		if err != nil {
			return fmt.Errorf("unit %s: %w", u.Name, err)
		}
		plan.Units = append(plan.Units, *pu)
		plan.Release = plan.Release || pu.Release
	}
	return nil
}

// plan1 plans one unit.
func (p *unitPlanner) plan1(ctx context.Context, u *spec.Unit) (*PlanUnit, error) {
	pu := &PlanUnit{Name: u.Name, Kind: u.Kind, Dir: u.Dir, Strategy: u.Strategy}
	if u.SkipPrereleases && p.branch.IsPrerelease() {
		pu.Reason = "the unit releases only from the release branch"
		return pu, nil
	}
	g := p.r.Git
	tags, err := g.FormatTags(ctx, "HEAD", u.TagFormat)
	if err != nil {
		return nil, err
	}
	last, from, err := p.start(u, pu, tags)
	if err != nil {
		return nil, err
	}

	all, err := g.CommitsWithFiles(ctx, from, "HEAD")
	if err != nil {
		return nil, err
	}
	a := &attribution{units: p.r.Config.Units, root: p.r.Config.RootPaths}
	raws := filterCommits(all, func(files []string) bool { return a.unitGets(u, files) })
	commits := ParseCommits(raws)
	bump := Analyze(DropSkipped(commits), p.r.Config.Rules)
	if bump == spec.BumpNone {
		pu.Reason = "no relevant changes in " + strings.Join(u.Paths, ", ")
		return pu, nil
	}

	next, bump, err := p.nextVersion(u, pu, last, tags, bump)
	if err != nil || next == nil {
		return pu, err
	}
	if u.Kind == spec.UnitGoModule {
		if err := spec.CheckGoMajor(u.Go.ModulePath, next); err != nil {
			return nil, err
		}
	}
	tag := u.TagFormat.Render(next.String())
	pu.Release = true
	pu.NextRelease = &PlanRelease{Type: bump, Version: next.String(), GitTag: tag, Channel: p.branch.Channel}
	prev := ""
	if pu.LastRelease != nil {
		prev = pu.LastRelease.GitTag
	}
	notes := RenderNotes(&NotesInput{
		Version: next.String(), Tag: tag, PreviousTag: prev, Date: p.now, Commits: commits, Links: p.links,
	})
	pu.Notes = maskerFor(ctx, p.opts.Env).Mask(notes)
	for _, c := range raws {
		pu.Commits = append(pu.Commits, c.Hash)
	}
	return pu, nil
}

// start finds the last release of a unit and the tag after which its
// commits start. An independent unit without a tag stops the plan, so
// that the first run does not release it from its whole history: the
// baseline command first tags the current version. When a unit has a
// tag, its manifest version does not count: the tag is the source of
// truth.
func (p *unitPlanner) start(u *spec.Unit, pu *PlanUnit, tags []Tag) (*Tag, string, error) {
	if last := LastRelease(p.branch, tags); last != nil {
		pu.LastRelease = &PlanRelease{Version: last.Version.String(), GitTag: last.Name}
		return last, last.Name, nil
	}
	if u.Strategy != spec.StrategyIndependent || len(tags) > 0 {
		return nil, "", nil
	}
	version, err := manifestVersion(p.r.Git.Dir, u)
	if err != nil {
		return nil, "", err
	}
	return nil, "", fmt.Errorf("%w: %s", ErrUnitNoTag, baselineHint(u, version, p.branch.Name))
}

// baselineHint tells how to make the first tag of u, whose manifest
// version is version, or "" when it has no manifest.
func baselineHint(u *spec.Unit, version, branch string) string {
	switch {
	case u.NPM == nil:
		return fmt.Sprintf(`run "devbase release baseline --unit %s --version <released version>" on %s `+
			"to tag the released version at the current commit", u.Name, branch)
	case !isReleaseVersion(version):
		return fmt.Sprintf("the version %q in %s/package.json is not a released version; set it to the "+
			`version on the registry, then run "devbase release baseline" on %s`, version, u.NPM.PkgRoot, branch)
	default:
		return fmt.Sprintf(`run "devbase release baseline" on %s to tag %s at the current commit, `+
			"so that releases start from it", branch, u.TagFormat.Render(version))
	}
}

// isReleaseVersion reports whether a manifest version is a strict
// semantic version and not a placeholder.
func isReleaseVersion(version string) bool {
	if slices.Contains(placeholderVersions, version) {
		return false
	}
	_, err := semver.StrictNewVersion(version)
	return err == nil
}

// manifestVersion returns the version in the package.json of an npm
// unit, or "" for the other kinds.
func manifestVersion(dir string, u *spec.Unit) (string, error) {
	if u.NPM == nil {
		return "", nil
	}
	pkg, err := readNPMPackage(filepath.Join(dir, filepath.FromSlash(u.NPM.PkgRoot)))
	if err != nil {
		return "", err
	}
	return pkg.Version, nil
}

// nextVersion returns the next version and the bump of a unit, or nil
// when a mirror unit does not release.
func (p *unitPlanner) nextVersion(
	u *spec.Unit, pu *PlanUnit, last *Tag, tags []Tag, bump spec.Bump,
) (*semver.Version, spec.Bump, error) {
	if u.Strategy == spec.StrategyIndependent {
		next := NextVersion(p.branch, last, tags, bump)
		if p.r.Config.MajorCheck && !u.AllowMajorVersions {
			if err := CheckMajor(last, next); err != nil {
				return nil, "", err
			}
		}
		return next, bump, nil
	}
	root := p.plan.NextRelease
	if root == nil {
		pu.Reason = "the root does not release"
		return nil, "", nil
	}
	next, err := ParseVersion(root.Version)
	if err != nil {
		return nil, "", err
	}
	for _, t := range tags {
		if !next.GreaterThan(t.Version) {
			return nil, "", fmt.Errorf("%w: %s is not higher than the tag %s", ErrUnitVersion, next, t.Name)
		}
	}
	return next, root.Type, nil
}
