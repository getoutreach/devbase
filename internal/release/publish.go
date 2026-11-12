// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes a planned release: checksums, hooks, the release
// commit, the tag and note, the publish step in plugin order, and the
// success step.

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the errors that Publish returns.
var (
	// ErrStalePlan is returned when the remote branch moved after the plan.
	ErrStalePlan = errors.New("the remote branch is not the plan commit")

	// ErrTagExists is returned when the plan's tag points to a commit that
	// is neither the plan commit nor its release commit.
	ErrTagExists = errors.New("the release tag exists on another commit")

	// ErrMissingAsset is returned when a build output from the plan is
	// missing.
	ErrMissingAsset = errors.New("a planned asset is missing")

	// ErrDryRunPlan is returned for a plan from plan --dry-run, unless
	// publish runs as a dry run too.
	ErrDryRunPlan = errors.New("the plan is a dry run; run plan without --dry-run to publish")

	// ErrNoForge is returned when the config has a forge plugin but
	// Publish got no forge.
	ErrNoForge = errors.New("the config publishes a forge release, but no forge is set")
)

// commitIdentity is the git identity of the release commit and the tag
// note, as semantic-release sets it unless the environment has one.
//
//nolint:gochecknoglobals // Why: read-only list.
var commitIdentity = []string{
	"GIT_AUTHOR_NAME=semantic-release-bot", "GIT_AUTHOR_EMAIL=semantic-release-bot@martynus.net",
	"GIT_COMMITTER_NAME=semantic-release-bot", "GIT_COMMITTER_EMAIL=semantic-release-bot@martynus.net",
}

// defaultCommitFiles are the files of the release commit when the commit
// plugin has no assets, as in @semantic-release/git.
//
//nolint:gochecknoglobals // Why: read-only list.
var defaultCommitFiles = []string{"CHANGELOG.md", "package.json", "package-lock.json", "npm-shrinkwrap.json"}

// PublishOptions are the inputs of Publish.
type PublishOptions struct {
	// Dist is the build output folder, relative to the repository.
	Dist string

	// Remote is the git URL to push to, with credentials.
	Remote string

	// Forge is the forge. It is used only when the config has a forge
	// plugin.
	Forge forge.Forge

	// Env is the environment of the hooks and of git.
	Env []string

	// Log receives progress messages and the output of the hooks.
	Log io.Writer

	// DryRun stops after the checks of the assets and of the tag, as
	// semantic-release --dry-run stops before the prepare step. It does
	// not report a failure to the forge.
	DryRun bool
}

// Publish publishes a planned release, in the order of the RFC:
// checksums, generateNotes and prepare hooks, the release commit, the tag
// and its git note, the publish step in plugin order, and the success
// step. A rerun after a failure continues the same release. When a step
// fails, the forge opens or updates the failure issue.
func Publish(ctx context.Context, r *Repo, plan *Plan, opts *PublishOptions) (err error) {
	switch {
	case !plan.Release:
		return nil
	case plan.DryRun && !opts.DryRun:
		return ErrDryRunPlan
	}
	if r.Config.Forge() != nil && opts.Forge == nil {
		return ErrNoForge
	}
	p := &publisher{r: r, plan: plan, opts: opts, log: logOr(opts.Log)}
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"assets", p.assets},
		{"state", p.state},
		{"generate_notes", p.generateNotes},
		{"prepare", p.prepare},
		{"tag", p.tag},
		{"publish", p.publish},
		{"success", p.success},
	}
	if opts.DryRun {
		steps = steps[:2]
	}
	defer func() {
		if err != nil && r.Config.Forge() != nil && !opts.DryRun {
			if ferr := opts.Forge.Fail(ctx, plan.Branch.Name, []error{err}); ferr != nil {
				err = errors.Join(err, fmt.Errorf("report the failure: %w", ferr))
			}
		}
	}()
	for _, step := range steps {
		if err := traceStep(ctx, "publish."+step.name, step.fn); err != nil {
			return err
		}
	}
	return nil
}

// publisher holds the state of one Publish call.
type publisher struct {
	r    *Repo
	plan *Plan
	opts *PublishOptions
	log  io.Writer

	// resume is true when the tag exists from an earlier run.
	resume bool

	// tagCommit is the commit to tag: the plan commit or its release
	// commit.
	tagCommit string

	// releases are the places where the release is available.
	releases []forge.Published
}

// assets checks that every planned asset exists, moves the archives of
// the build jobs from dist/<os>_<arch>/ to dist/, so that the forge
// plugin's globs such as "dist/*.tar.gz" find them, and writes the
// checksum file (requirement 11).
func (p *publisher) assets(context.Context) error {
	a := p.r.Config.Artifacts
	if a == nil {
		return nil
	}
	dist := filepath.Join(p.r.Git.Dir, p.opts.Dist)
	var archives []string
	for _, asset := range p.plan.Assets {
		if asset == a.Checksums {
			continue
		}
		file := filepath.Join(dist, asset)
		if _, err := os.Stat(file); err != nil {
			matches, err := filepath.Glob(filepath.Join(dist, "*", asset))
			if err != nil || len(matches) != 1 {
				return fmt.Errorf("%w: %s", ErrMissingAsset, asset)
			}
			if err := os.Rename(matches[0], file); err != nil {
				return fmt.Errorf("move %s: %w", asset, err)
			}
		}
		archives = append(archives, file)
	}
	return WriteChecksums(filepath.Join(dist, a.Checksums), archives)
}

// state finds whether an earlier run made the tag (requirement 8), and
// fetches the tags and notes for that.
func (p *publisher) state(ctx context.Context) error {
	g := p.r.Git
	if err := g.Fetch(ctx, p.opts.Remote, p.plan.Branch.Name, p.plan.Branch.Name); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	g.FetchNotes(ctx, p.opts.Remote)
	p.tagCommit = p.plan.Commit
	tagged, err := g.Run(ctx, "rev-parse", "--verify", "--quiet", "refs/tags/"+p.plan.NextRelease.GitTag+"^{commit}")
	if err != nil {
		return nil //nolint:nilerr // Why: rev-parse fails when there is no such tag.
	}
	if tagged != p.plan.Commit && !p.isReleaseCommit(ctx, tagged) {
		return fmt.Errorf("%w: %s at %s", ErrTagExists, p.plan.NextRelease.GitTag, tagged)
	}
	p.resume, p.tagCommit = true, tagged
	fmt.Fprintf(p.log, "Continue the release of %s from an earlier run\n", p.plan.NextRelease.GitTag)
	return nil
}

// isReleaseCommit reports whether commit is a release commit of the plan:
// its parent is the plan commit and it has the release commit message.
func (p *publisher) isReleaseCommit(ctx context.Context, commit string) bool {
	out, err := p.r.Git.Run(ctx, "log", "-1", "--format=%P%x00%B", commit)
	parent, msg, _ := strings.Cut(out, "\x00")
	return err == nil && parent == p.plan.Commit && strings.TrimSpace(msg) == strings.TrimSpace(p.commitMessage())
}

// commitMessage is the release commit message of @semantic-release/git.
func (p *publisher) commitMessage() string {
	return releaseCommitSubject(p.plan.NextRelease.Version) + "\n\n" + p.plan.Notes
}

// releaseCommitSubject is the subject of the release commit of version.
func releaseCommitSubject(version string) string {
	return "chore(release): " + version + " [skip ci]"
}

// generateNotes runs the exec generateNotes hooks, in plugin order. Their
// stdout does not change the planned notes.
func (p *publisher) generateNotes(ctx context.Context) error {
	for _, pl := range p.r.Config.Plugins {
		if pl.Exec != nil && pl.Exec.GenerateNotes != "" {
			if _, err := p.hook(ctx, pl.Exec.GenerateNotes); err != nil {
				return fmt.Errorf("generateNotes hook: %w", err)
			}
		}
	}
	return nil
}

// prepare runs the prepare step in plugin order: exec prepare hooks, the
// npm version, and the release commit. An earlier run that tagged the
// release already made its release commit.
func (p *publisher) prepare(ctx context.Context) error {
	for _, pl := range p.r.Config.Plugins {
		switch {
		case pl.Exec != nil && pl.Exec.Prepare != "":
			if _, err := p.hook(ctx, pl.Exec.Prepare); err != nil {
				return fmt.Errorf("prepare hook: %w", err)
			}
		case isRegistry(&pl):
			if err := registryOf(&pl).prepare(ctx, p.target()); err != nil {
				return err
			}
		case pl.Commit != nil && !p.resume:
			if err := p.releaseCommit(ctx, pl.Commit); err != nil {
				return err
			}
		}
	}
	return nil
}

// releaseCommit commits the changed files that match the commit plugin's
// globs, as @semantic-release/git does, except the client version files
// (requirement 12). The tag then points to it. It pushes the commit
// after the stale plan check.
func (p *publisher) releaseCommit(ctx context.Context, c *spec.CommitPlugin) error {
	globs := c.Files
	if globs == nil {
		globs = defaultCommitFiles
	}
	out, err := p.r.Git.Run(ctx, "ls-files", "-m", "-o")
	if err != nil {
		return err
	}
	var files []string
	for f := range strings.FieldsSeq(out) {
		if !slices.ContainsFunc(globs, func(g string) bool { return matchCommitGlob(g, f) }) {
			continue
		}
		if p.r.Config.IsClientVersionFile(f) {
			fmt.Fprintf(p.log, "Leave the client version file %s out of the release commit\n", f)
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil
	}
	env := slices.Concat(commitIdentity, p.opts.Env)
	if _, err := run(ctx, p.r.Git.Dir, env, "git", append([]string{"add", "--force", "--"}, files...)...); err != nil {
		return err
	}
	if _, err := run(ctx, p.r.Git.Dir, env, "git", "commit", "-q", "-m", p.commitMessage()); err != nil {
		return err
	}
	if p.tagCommit, err = p.r.Git.Run(ctx, "rev-parse", "HEAD"); err != nil {
		return err
	}
	fmt.Fprintf(p.log, "Made the release commit %s with %d file(s)\n", p.tagCommit, len(files))
	return nil
}

// matchCommitGlob reports whether file matches a release commit glob: a
// path.Match pattern, or a folder that contains the file.
func matchCommitGlob(glob, file string) bool {
	glob = strings.TrimSuffix(strings.TrimPrefix(glob, "./"), "/")
	ok, err := path.Match(glob, file)
	return (err == nil && ok) || strings.HasPrefix(file, glob+"/")
}

// tag checks that the plan is not stale (requirement 7), pushes the
// release commit, and tags the release and adds its git note, as
// semantic-release does. A rerun skips the stale check, because the
// remote branch can then be the release commit.
func (p *publisher) tag(ctx context.Context) error {
	g, tag, branch := p.r.Git, p.plan.NextRelease.GitTag, p.plan.Branch.Name
	if !p.resume {
		head, err := g.RemoteHead(ctx, p.opts.Remote, branch)
		if err != nil {
			return err
		}
		if head != p.plan.Commit {
			return fmt.Errorf("%w: %s is at %s, the plan is for %s", ErrStalePlan, branch, head, p.plan.Commit)
		}
		if p.tagCommit != p.plan.Commit {
			if _, err := g.Run(ctx, "push", "--", p.opts.Remote, p.tagCommit+":refs/heads/"+branch); err != nil {
				return fmt.Errorf("push the release commit: %w", err)
			}
		}
		if _, err := g.Run(ctx, "tag", tag, p.tagCommit); err != nil {
			return err
		}
	}
	var channel any // A JSON null for the default channel.
	if c := p.plan.NextRelease.Channel; c != "" {
		channel = c
	}
	note, err := json.Marshal(map[string][]any{"channels": {channel}})
	if err != nil {
		return fmt.Errorf("encode the tag note: %w", err)
	}
	env := slices.Concat(commitIdentity, p.opts.Env)
	ref := noteRef(tag)
	if _, err := run(ctx, g.Dir, env, "git", "notes", "--ref", ref, "add", "-f", "-m", string(note), tag); err != nil {
		return err
	}
	if _, err := g.Run(ctx, "push", "--", p.opts.Remote, "refs/tags/"+tag); err != nil {
		return fmt.Errorf("push the tag: %w", err)
	}
	if _, err := g.Run(ctx, "push", "--", p.opts.Remote, ref); err != nil {
		return fmt.Errorf("push the tag note: %w", err)
	}
	fmt.Fprintf(p.log, "Tagged %s as %s\n", p.tagCommit, tag)
	return nil
}

// publish runs the publish step in plugin order.
func (p *publisher) publish(ctx context.Context) error {
	for _, pl := range p.r.Config.Plugins {
		switch {
		case pl.Exec != nil && pl.Exec.Publish != "":
			out, err := p.hook(ctx, pl.Exec.Publish)
			if err != nil {
				return fmt.Errorf("publish hook: %w", err)
			}
			if info := execRelease(out); info != nil {
				p.releases = append(p.releases, *info)
			}
		case pl.Forge != nil:
			if err := p.forgeRelease(ctx, pl.Forge); err != nil {
				return err
			}
		case isRegistry(&pl):
			info, err := registryOf(&pl).publish(ctx, p.target())
			if err != nil {
				return err
			}
			if info != nil {
				p.releases = append(p.releases, *info)
			}
		}
	}
	return nil
}

// registry publishes a package to a package registry.
type registry interface {
	// name names the registry in messages.
	name() string

	// prepare runs in the prepare step, such as setting the package
	// version in the checkout.
	prepare(ctx context.Context, t *regTarget) error

	// publish runs in the publish step. It skips the publish when the
	// registry has the version (requirement 8), and returns where the
	// release is available, or nil.
	publish(ctx context.Context, t *regTarget) (*forge.Published, error)

	// exists reports whether the registry has the version.
	exists(ctx context.Context, t *regTarget) (bool, error)
}

// regTarget is the release that a registry prepares, publishes or looks
// up.
type regTarget struct {
	// dir is the repository directory.
	dir string

	// version and channel are the version and channel of the release.
	version, channel string

	// env is the environment of the registry commands, or nil for the
	// process environment.
	env []string

	// log receives progress messages and command output.
	log io.Writer
}

// target returns the registry target of the publish.
func (p *publisher) target() *regTarget {
	return &regTarget{
		dir: p.r.Git.Dir, version: p.plan.NextRelease.Version, channel: p.plan.NextRelease.Channel,
		env: p.opts.Env, log: p.log,
	}
}

// isRegistry reports whether pl is an npm or orb plugin.
func isRegistry(pl *spec.Plugin) bool {
	return pl.NPM != nil || pl.Orb != nil
}

// registryOf returns the registry of an npm or orb plugin.
func registryOf(pl *spec.Plugin) registry {
	if pl.NPM != nil {
		return newNPMRegistry(pl.NPM)
	}
	return newOrbRegistry(pl.Orb)
}

// logOr returns w, or io.Discard when w is nil.
func logOr(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

// execRelease reads the release information that an exec publish hook
// prints as a JSON object, as @semantic-release/exec does, or nil.
func execRelease(stdout string) *forge.Published {
	var info forge.Published
	if err := json.Unmarshal([]byte(stdout), &info); err != nil || info.Name == "" && info.URL == "" {
		return nil
	}
	return &info
}

// forgeRelease publishes the forge release with its assets, as
// @semantic-release/github does: a release with assets is a draft until
// they are uploaded. A rerun continues a draft: it replaces broken or
// changed uploads and uploads the missing assets.
func (p *publisher) forgeRelease(ctx context.Context, fp *spec.ForgePlugin) error {
	f, next := p.opts.Forge, p.plan.NextRelease
	files, err := p.globAssets(fp.Assets)
	if err != nil {
		return err
	}
	// A draft from an earlier run exists only after its tag push.
	var rel *forge.Release
	if p.resume {
		if rel, err = f.FindRelease(ctx, next.GitTag); err != nil {
			return err
		}
	}
	latest := !p.plan.Branch.IsPrerelease()
	if rel == nil {
		rel, err = f.CreateRelease(ctx, &forge.ReleaseSpec{
			Tag: next.GitTag, Target: p.plan.Branch.Name, Name: next.GitTag, Notes: p.plan.Notes,
			Prerelease: p.plan.Branch.IsPrerelease(), Latest: latest, Draft: len(files) > 0,
		})
		if err != nil {
			return err
		}
	}
	if rel, err = finishRelease(ctx, f, rel, files, latest); err != nil {
		return err
	}
	fmt.Fprintf(p.log, "Published %s: %s\n", f.ReleaseName(), rel.URL)
	p.releases = append(p.releases, forge.Published{Name: f.ReleaseName(), URL: rel.URL})
	return nil
}

// globAssets returns the files that match the forge plugin's globs,
// relative to the repository, without duplicates.
func (p *publisher) globAssets(globs []string) ([]string, error) {
	var files []string
	for _, g := range globs {
		matches, err := filepath.Glob(filepath.Join(p.r.Git.Dir, g))
		if err != nil {
			return nil, fmt.Errorf("asset glob %q: %w", g, err)
		}
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() && !slices.Contains(files, m) {
				files = append(files, m)
			}
		}
	}
	return files, nil
}

// finishRelease uploads the files to a draft release and publishes it.
// An asset that an earlier run uploaded in full, with the same size, is
// kept; a broken or changed one is replaced. A published release is
// returned as it is.
func finishRelease(ctx context.Context, f forge.Forge, rel *forge.Release, files []string, latest bool) (*forge.Release, error) {
	if !rel.Draft {
		return rel, nil
	}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", file, err)
		}
		i := slices.IndexFunc(rel.Assets, func(a forge.Asset) bool { return a.Name == filepath.Base(file) })
		if i >= 0 {
			if a := &rel.Assets[i]; a.Complete && a.Size == info.Size() {
				continue
			} else if err := f.DeleteAsset(ctx, rel, a); err != nil {
				return nil, err
			}
		}
		if _, err := f.UploadAsset(ctx, rel, file); err != nil {
			return nil, err
		}
	}
	return f.PublishRelease(ctx, rel, latest)
}

// success runs the success step in plugin order: the forge comments and
// labels, and the exec success hooks.
func (p *publisher) success(ctx context.Context) error {
	commits, err := p.commits(ctx)
	if err != nil {
		return err
	}
	for _, pl := range p.r.Config.Plugins {
		switch {
		case pl.Exec != nil && pl.Exec.Success != "":
			if _, err := p.hook(ctx, pl.Exec.Success); err != nil {
				return fmt.Errorf("success hook: %w", err)
			}
		case pl.Forge != nil:
			if err := p.opts.Forge.Success(ctx, &forge.Success{
				Version: p.plan.NextRelease.Version, Channel: p.plan.NextRelease.Channel,
				Commits: commits, Releases: p.releases,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// commits returns the commits of the release, from the last release to
// the plan commit.
func (p *publisher) commits(ctx context.Context) ([]forge.Commit, error) {
	from := ""
	if p.plan.LastRelease != nil {
		from = p.plan.LastRelease.GitTag
	}
	raws, err := p.r.Git.Commits(ctx, from, p.plan.Commit)
	if err != nil {
		return nil, err
	}
	out := make([]forge.Commit, len(raws))
	for i, c := range raws {
		out[i] = forge.Commit{Hash: c.Hash, Message: c.Message}
	}
	return out, nil
}

// hook runs an exec command of the publish, and returns its stdout.
func (p *publisher) hook(ctx context.Context, cmd string) (string, error) {
	return runExecHook(ctx, p.r.Git.Dir, p.opts.Env, p.log, cmd, p.plan.execFields())
}
