// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes the release units of a plan: the unit tags and
// notes in one push, the npm packages, the forge releases, the root
// go.mod pin and the success comments.

package release

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// ErrUnitPlan is returned for a plan unit that the config does not have.
var ErrUnitPlan = errors.New("the plan has a unit that the config does not have")

// unitPublish is the publish state of one released unit.
type unitPublish struct {
	unit *spec.Unit
	plan *PlanUnit

	// done is true when an earlier run pushed the tag.
	done bool

	// failed is true when a publisher of the unit failed.
	failed bool

	// releases are the places where the release is available.
	releases []forge.Published
}

// tag returns the tag of the unit release.
func (u *unitPublish) tag() string {
	return u.plan.NextRelease.GitTag
}

// unitState finds the released units of the plan in the config, and
// whether an earlier run tagged them (requirement 8). A unit tag on
// another commit than the plan commit is an error.
func (p *publisher) unitState(ctx context.Context) error {
	for _, pu := range p.plan.ReleasedUnits() {
		u := p.r.Config.Unit(pu.Name)
		if u == nil || u.TagFormat.Render(pu.NextRelease.Version) != pu.NextRelease.GitTag {
			return fmt.Errorf("%w: %s", ErrUnitPlan, pu.Name)
		}
		up := &unitPublish{unit: u, plan: pu}
		if tagged := p.r.Git.TagCommit(ctx, up.tag()); tagged != "" {
			if tagged != p.plan.Commit {
				return fmt.Errorf("%w: %s at %s", ErrTagExists, up.tag(), tagged)
			}
			up.done = true
			fmt.Fprintf(p.log, "Continue the release of %s from an earlier run\n", up.tag())
		}
		p.units = append(p.units, up)
	}
	return nil
}

// tagUnits tags each released unit on the plan commit, adds the git
// notes, and pushes all of them in one atomic push. It first checks
// that the plan is not stale, unless an earlier run of this plan pushed
// a tag, after which the branch can have the release commit.
func (p *publisher) tagUnits(ctx context.Context) error {
	if len(p.units) == 0 {
		return nil
	}
	g := p.r.Git
	if !p.resume && !slices.ContainsFunc(p.units, func(u *unitPublish) bool { return u.done }) {
		if err := p.checkStale(ctx); err != nil {
			return err
		}
	}
	tags := make([]string, 0, len(p.units))
	for _, u := range p.units {
		tags = append(tags, u.tag())
	}
	if err := tagAndPush(ctx, g, p.opts.Env, p.opts.Remote, p.plan.Commit, p.plan.Branch.Channel, tags); err != nil {
		return err
	}
	for _, u := range p.units {
		u.done = true
		fmt.Fprintf(p.log, "Tagged %s as %s\n", p.plan.Commit, u.tag())
	}
	return nil
}

// publishUnits publishes each released unit in config order: the npm
// package and the forge release. A unit that fails does not stop the
// other units or the root release; Publish returns its error at the end.
func (p *publisher) publishUnits(ctx context.Context) error {
	for _, u := range p.units {
		if err := p.publishUnit(ctx, u); err != nil {
			u.failed = true
			p.unitErrs = append(p.unitErrs, fmt.Errorf("unit %s: %w", u.unit.Name, err))
			fmt.Fprintf(p.log, "Failed to publish %s: %v\n", u.tag(), err)
		}
	}
	return nil
}

// publishUnit publishes one unit.
func (p *publisher) publishUnit(ctx context.Context, u *unitPublish) error {
	if pl := u.unit.NPM; pl != nil {
		reg := newNPMRegistry(pl)
		t := p.unitTarget(u)
		if err := reg.prepare(ctx, t); err != nil {
			return err
		}
		info, err := reg.publish(ctx, t)
		if err != nil {
			return err
		}
		if info != nil {
			u.releases = append(u.releases, *info)
		}
	}
	if u.unit.ForgeRelease {
		return p.unitForgeRelease(ctx, u)
	}
	return nil
}

// unitTarget returns the registry target of a unit release.
func (p *publisher) unitTarget(u *unitPublish) *regTarget {
	next := u.plan.NextRelease
	return &regTarget{
		dir: p.r.Git.Dir, version: next.Version, channel: next.Channel, env: p.opts.Env, exec: p.opts.Exec, log: p.log,
	}
}

// unitForgeRelease makes the forge release of a unit, or finds the one
// of an earlier run. It has no assets and is never the latest release of
// the repository, which stays the root release.
func (p *publisher) unitForgeRelease(ctx context.Context, u *unitPublish) error {
	f := p.opts.Forge
	rel, err := f.FindRelease(ctx, u.tag())
	if err != nil {
		return err
	}
	if rel == nil {
		rel, err = f.CreateRelease(ctx, &forge.ReleaseSpec{
			Tag: u.tag(), Target: p.plan.Branch.Name, Name: p.unitReleaseName(u), Notes: u.plan.Notes,
			Prerelease: p.plan.Branch.IsPrerelease(), Latest: false,
		})
		if err != nil {
			return err
		}
	}
	if rel, err = finishRelease(ctx, f, rel, nil, false); err != nil {
		return err
	}
	fmt.Fprintf(p.log, "Published %s: %s\n", f.ReleaseName(), rel.URL)
	u.releases = append(u.releases, forge.Published{Name: f.ReleaseName(), URL: rel.URL})
	return nil
}

// unitReleaseName is the name of a unit's forge release: the package
// name and the version for an npm unit, as the releases that people made
// by hand, or else the tag.
func (p *publisher) unitReleaseName(u *unitPublish) string {
	if u.unit.NPM != nil {
		return p.unitDisplayName(u.unit) + " v" + u.plan.NextRelease.Version
	}
	return u.tag()
}

// unitDisplayName names a unit in success comments: the npm package
// name, the Go module path, or the unit name.
func (p *publisher) unitDisplayName(u *spec.Unit) string {
	switch {
	case u.NPM != nil:
		if pkg, err := readNPMPackage(filepath.Join(p.r.Git.Dir, filepath.FromSlash(u.NPM.PkgRoot))); err == nil {
			return pkg.Name
		}
	case u.Go != nil:
		return u.Go.ModulePath
	}
	return u.Name
}

// unitSuccess runs the forge success step of each unit that published,
// with the commits of the unit and the unit name in the comments.
func (p *publisher) unitSuccess(ctx context.Context) error {
	if p.r.Config.Forge() == nil {
		return nil
	}
	for _, u := range p.units {
		if u.failed {
			continue
		}
		commits, err := p.unitCommits(ctx, u.plan)
		if err != nil {
			return err
		}
		err = p.opts.Forge.Success(ctx, &forge.Success{
			Unit: p.unitDisplayName(u.unit), Version: u.plan.NextRelease.Version,
			Channel: u.plan.NextRelease.Channel, Commits: commits, Releases: u.releases,
		})
		if err != nil {
			return fmt.Errorf("unit %s: %w", u.unit.Name, err)
		}
	}
	return nil
}

// unitCommits returns the commits of a unit release that the plan
// lists, with their messages.
func (p *publisher) unitCommits(ctx context.Context, pu *PlanUnit) ([]forge.Commit, error) {
	from := ""
	if pu.LastRelease != nil {
		from = pu.LastRelease.GitTag
	}
	raws, err := p.r.Git.Commits(ctx, from, p.plan.Commit)
	if err != nil {
		return nil, err
	}
	var out []forge.Commit
	for _, c := range raws {
		if slices.Contains(pu.Commits, c.Hash) {
			out = append(out, forge.Commit{Hash: c.Hash, Message: c.Message})
		}
	}
	return out, nil
}

// pinGoUnits sets the root go.mod requirement of each released Go unit
// to its new version, before the release commit: with a local replace
// directive, an offline go mod edit is enough; without one, go get needs
// the unit tag, which the units step pushed, and go mod tidy updates
// go.sum.
func (p *publisher) pinGoUnits(ctx context.Context) error {
	dir, env := p.r.Git.Dir, p.opts.Env
	for _, u := range p.units {
		g := u.unit.Go
		if g == nil || g.PinRoot != spec.GoPinAuto || !g.RootRequires {
			continue
		}
		mod := g.ModulePath + "@v" + u.plan.NextRelease.Version
		cmds := [][]string{{"get", mod}, {"mod", "tidy"}}
		if g.RootReplaces {
			cmds = [][]string{{"mod", "edit", "-require=" + mod}}
		}
		for _, args := range cmds {
			if _, err := run(ctx, dir, env, "go", args...); err != nil {
				return fmt.Errorf("pin %s in go.mod: %w", mod, err)
			}
		}
		fmt.Fprintf(p.log, "Set the go.mod requirement %s\n", mod)
	}
	return nil
}
