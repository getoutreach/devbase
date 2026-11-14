// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes the release units of a plan: the unit tags and
// notes, the npm packages, the forge releases, the root go.mod pin and
// the success comments.

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// ErrUnitPlan is returned for a plan unit that the config does not have.
var ErrUnitPlan = errors.New("the plan has a unit that the config does not have")

// unitPublish is the publish state of one released unit.
type unitPublish struct {
	unit *spec.Unit
	plan *PlanUnit

	// done is true when the tag is on the plan commit: an earlier run or
	// the units step pushed it.
	done bool

	// early is true when the units step pushes the tag, before the root
	// tag: for an independent unit, and for a mirror Go unit whose root
	// pin runs go get, which needs the tag on the remote. The tag push
	// of the root release pushes the other mirror unit tags.
	early bool

	// failed is true when a publisher of the unit failed.
	failed bool

	// releases are the places where the release is available.
	releases []forge.Published
}

// tag returns the tag of the unit release.
func (u *unitPublish) tag() string {
	return u.plan.NextRelease.GitTag
}

// mirror reports whether the unit gets the root version.
func (u *unitPublish) mirror() bool {
	return u.plan.Strategy == spec.StrategyMirror
}

// pinsWithGoGet reports whether the root go.mod pin of a Go unit runs go
// get, because the root go.mod requires the unit without a local
// replace directive.
func (u *unitPublish) pinsWithGoGet() bool {
	g := u.unit.Go
	return g != nil && g.PinRoot == spec.GoPinAuto && g.RootRequires && !g.RootReplaces
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
		up.early = !up.mirror() || up.pinsWithGoGet()
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

// tagUnits tags each early unit on the plan commit, adds the git notes,
// and pushes all of them in one atomic push. It first checks that the
// plan is not stale, unless an earlier run of this plan pushed a tag,
// after which the branch can have the release commit.
func (p *publisher) tagUnits(ctx context.Context) error {
	units := p.unitsWhere(func(u *unitPublish) bool { return u.early })
	if len(units) == 0 {
		return nil
	}
	if !p.resume && !slices.ContainsFunc(units, func(u *unitPublish) bool { return u.done }) {
		if err := p.checkStale(ctx); err != nil {
			return err
		}
	}
	refs, err := p.tagUnitRefs(ctx, units)
	if err != nil {
		return err
	}
	if err := pushAtomic(ctx, p.r.Git, p.opts.Remote, refs); err != nil {
		return err
	}
	p.markTagged(units)
	return nil
}

// markTagged marks units as tagged after their push.
func (p *publisher) markTagged(units []*unitPublish) {
	for _, u := range units {
		u.done = true
		fmt.Fprintf(p.log, "Tagged %s as %s\n", p.plan.Commit, u.tag())
	}
}

// unitsWhere returns the released units for which keep is true.
func (p *publisher) unitsWhere(keep func(*unitPublish) bool) []*unitPublish {
	var out []*unitPublish
	for _, u := range p.units {
		if keep(u) {
			out = append(out, u)
		}
	}
	return out
}

// tagUnitRefs tags units on the plan commit with their git notes, and
// returns the refs to push.
func (p *publisher) tagUnitRefs(ctx context.Context, units []*unitPublish) ([]string, error) {
	tags := make([]string, 0, len(units))
	for _, u := range units {
		tags = append(tags, u.tag())
	}
	return tagRefs(ctx, p.r.Git, p.opts.Env, p.plan.Commit, p.plan.Branch.Channel, tags)
}

// dropEarlyMirrorTags deletes the tags and notes of the mirror units
// that the units step pushed, after a root step failed before the root
// tag push. A mirror unit tag without its root release stops the later
// plans. The go get of the root pin did not use a module proxy or
// checksum database (see goGetEnv), so no public cache has the version.
func (p *publisher) dropEarlyMirrorTags(ctx context.Context) error {
	units := p.unitsWhere(func(u *unitPublish) bool { return u.early && u.mirror() && u.done })
	if len(units) == 0 {
		return nil
	}
	tags := make([]string, 0, len(units))
	refs := make([]string, 0, 2*len(units)) //nolint:mnd // Why: a tag and a note per unit.
	for _, u := range units {
		tags = append(tags, u.tag())
		refs = append(refs, ":refs/tags/"+u.tag(), ":"+noteRef(u.tag()))
	}
	if err := pushAtomic(ctx, p.r.Git, p.opts.Remote, refs); err != nil {
		return fmt.Errorf("delete the unit tags %s of the failed root release; run rollback --force for each: %w",
			strings.Join(tags, ", "), err)
	}
	for _, u := range units {
		u.done = false
		deleteLocalTag(ctx, p.r.Git, u.tag())
	}
	fmt.Fprintf(p.log, "Deleted the unit tags %s, because the root release failed\n", strings.Join(tags, ", "))
	return nil
}

// publishUnits publishes each released unit in config order: the npm
// package and the forge release. It publishes the mirror units when
// mirror is true, and else the other units. A unit that fails does not
// stop the other units or the root release; Publish returns its error at
// the end.
func (p *publisher) publishUnits(ctx context.Context, mirror bool) error {
	for _, u := range p.units {
		if u.mirror() != mirror {
			continue
		}
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
	dir := p.r.Git.Dir
	for _, u := range p.units {
		g := u.unit.Go
		if g == nil || g.PinRoot != spec.GoPinAuto || !g.RootRequires {
			continue
		}
		mod := g.ModulePath + "@v" + u.plan.NextRelease.Version
		cmds, env := [][]string{{"mod", "edit", "-require=" + mod}}, p.opts.Env
		if u.pinsWithGoGet() {
			cmds = [][]string{{"get", mod}, {"mod", "tidy"}}
			var err error
			if env, err = goGetEnv(ctx, dir, env, g.ModulePath); err != nil {
				return err
			}
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

// goGetEnv returns env with module added to GONOPROXY and GONOSUMDB, so
// that go get fetches the new unit version from the git remote, and no
// module proxy or checksum database gets it before the root release.
// Then publish can delete the unit tag if the root release fails.
func goGetEnv(ctx context.Context, dir string, env []string, module string) ([]string, error) {
	keys := []string{"GONOPROXY", "GONOSUMDB"}
	out, err := run(ctx, dir, env, "go", append([]string{"env", "-json"}, keys...)...)
	if err != nil {
		return nil, fmt.Errorf("read the go env: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(out), &values); err != nil {
		return nil, fmt.Errorf("read the go env: %w", err)
	}
	env = slices.Clone(envOrProcess(env))
	for _, key := range keys {
		v := module
		if values[key] != "" {
			v = values[key] + "," + module
		}
		env = append(env, key+"="+v)
	}
	return env, nil
}
