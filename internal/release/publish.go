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
	"github.com/getoutreach/gobox/pkg/box"
	"github.com/getoutreach/gobox/pkg/exec"
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

	// Exec creates the registry commands, such as npm, bundle, gem,
	// poetry, aws, circleci and mise. Nil means real processes. git, go,
	// the builder and the exec hooks always run as real processes.
	Exec exec.Exec

	// Log receives progress messages and the output of the hooks.
	Log io.Writer

	// DryRun stops after the checks of the assets and of the tag, and
	// the build checks of the registries, such as the gem build. It
	// makes no release commit, tag, push or registry publish, and does
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
	// The root steps run only when the root releases.
	type step struct {
		name string
		fn   func(context.Context) error
		root bool
	}
	steps := []step{
		{"assets", p.assets, true},
		{"state", p.state, false},
		{"generate_notes", p.generateNotes, true},
		{"units", p.tagUnits, false},
		{"unit_publish", p.publishUnits, false},
		{"prepare", p.prepare, true},
		{"tag", p.tag, true},
		{"publish", p.publish, true},
		{"success", p.success, false},
	}
	if opts.DryRun {
		steps = append(steps[:2:2], step{"build_check", p.checkBuilds, true})
	}
	defer func() {
		if err != nil && r.Config.Forge() != nil && !opts.DryRun {
			if ferr := opts.Forge.Fail(ctx, plan.Branch.Name, []error{err}); ferr != nil {
				err = errors.Join(err, fmt.Errorf("report the failure: %w", ferr))
			}
		}
	}()
	for _, step := range steps {
		if step.root && !plan.RootRelease() {
			continue
		}
		if err := traceStep(ctx, "publish."+step.name, step.fn); err != nil {
			return err
		}
	}
	// A unit that failed to publish did not stop the other releases.
	return errors.Join(p.unitErrs...)
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

	// units holds the publish state of the released units.
	units []*unitPublish

	// unitErrs are the errors of the units that failed to publish.
	unitErrs []error
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
	if err := p.unitState(ctx); err != nil {
		return err
	}
	if !p.plan.RootRelease() {
		return nil
	}
	p.tagCommit = p.plan.Commit
	tagged := g.TagCommit(ctx, p.plan.NextRelease.GitTag)
	if tagged == "" {
		return nil
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
	if !p.resume {
		if err := p.pinGoUnits(ctx); err != nil {
			return err
		}
	}
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
		if err := p.checkStale(ctx); err != nil {
			return err
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
	if err := addTagNote(ctx, g, p.opts.Env, tag, p.plan.NextRelease.Channel); err != nil {
		return err
	}
	ref := noteRef(tag)
	if _, err := g.Run(ctx, "push", "--", p.opts.Remote, "refs/tags/"+tag); err != nil {
		return fmt.Errorf("push the tag: %w", err)
	}
	if _, err := g.Run(ctx, "push", "--", p.opts.Remote, ref); err != nil {
		return fmt.Errorf("push the tag note: %w", err)
	}
	fmt.Fprintf(p.log, "Tagged %s as %s\n", p.tagCommit, tag)
	return nil
}

// checkStale checks that the remote branch is the plan commit
// (requirement 7).
func (p *publisher) checkStale(ctx context.Context) error {
	branch := p.plan.Branch.Name
	head, err := p.r.Git.RemoteHead(ctx, p.opts.Remote, branch)
	if err != nil {
		return err
	}
	if head != p.plan.Commit {
		return fmt.Errorf("%w: %s is at %s, the plan is for %s", ErrStalePlan, branch, head, p.plan.Commit)
	}
	return nil
}

// addTagNote adds the git note of a release tag with its channel, as
// semantic-release does: {"channels":[null]} for the default channel.
func addTagNote(ctx context.Context, g *Git, env []string, tag, channel string) error {
	var ch any // A JSON null for the default channel.
	if channel != "" {
		ch = channel
	}
	note, err := json.Marshal(map[string][]any{"channels": {ch}})
	if err != nil {
		return fmt.Errorf("encode the tag note: %w", err)
	}
	_, err = run(ctx, g.Dir, slices.Concat(commitIdentity, env), "git", "notes", "--ref", noteRef(tag), "add", "-f", "-m", string(note), tag)
	return err
}

// tagAndPush tags commit with each of tags that does not exist yet, adds
// the git notes with channel, and pushes the tags and notes to remote
// in one atomic push, unless remote is empty.
func tagAndPush(ctx context.Context, g *Git, env []string, remote, commit, channel string, tags []string) error {
	refs := make([]string, 0, 2*len(tags)) //nolint:mnd // Why: a tag and a note per tag.
	for _, tag := range tags {
		if g.TagCommit(ctx, tag) == "" {
			if _, err := g.Run(ctx, "tag", tag, commit); err != nil {
				return err
			}
		}
		if err := addTagNote(ctx, g, env, tag, channel); err != nil {
			return err
		}
		refs = append(refs, "refs/tags/"+tag, noteRef(tag))
	}
	if remote == "" {
		return nil
	}
	if _, err := g.Run(ctx, append([]string{"push", "--atomic", "--", remote}, refs...)...); err != nil {
		return fmt.Errorf("push the tags %s: %w", strings.Join(tags, ", "), err)
	}
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

// checkBuilds runs the build checks of the registries in the
// repository, with the planned version.
func (p *publisher) checkBuilds(ctx context.Context) error {
	return checkBuilds(ctx, p.r.Config, p.target())
}

// checkBuilds runs the build check of each registry plugin of cfg that
// has one, in plugin order.
func checkBuilds(ctx context.Context, cfg *spec.Config, t *regTarget) error {
	for _, pl := range cfg.Plugins {
		if !isRegistry(&pl) {
			continue
		}
		if c, ok := registryOf(&pl).(buildChecker); ok {
			if err := c.checkBuild(ctx, t); err != nil {
				return fmt.Errorf("build check: %w", err)
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

	// exists reports whether the registry has the version. It reports a
	// missing version only after it shows that it reached the right
	// registry with valid credentials, because rollback trusts it. The
	// npm registry checks the token only on the default registry, as
	// prepare does.
	exists(ctx context.Context, t *regTarget) (bool, error)
}

// buildChecker is a registry that can build its package without
// publishing it, for the dry runs.
type buildChecker interface {
	// checkBuild builds the package of version t.version and checks the
	// build output, without publishing it. It needs no publish
	// credentials, and restores the files that it changes to set the
	// version.
	checkBuild(ctx context.Context, t *regTarget) error
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

	// exec creates the registry commands, or is nil for real processes.
	exec exec.Exec

	// log receives progress messages and command output.
	log io.Writer
}

// run runs a registry command with the Exec of t.
func (t *regTarget) run(ctx context.Context, log io.Writer, c *command) (cmdResult, error) {
	return execCommand(ctx, t.exec, log, c)
}

// target returns the registry target of the publish.
func (p *publisher) target() *regTarget {
	return &regTarget{
		dir: p.r.Git.Dir, version: p.plan.NextRelease.Version, channel: p.plan.NextRelease.Channel,
		env: p.opts.Env, exec: p.opts.Exec, log: p.log,
	}
}

// isRegistry reports whether pl is an npm, orb, gem or Python plugin.
func isRegistry(pl *spec.Plugin) bool {
	return pl.NPM != nil || pl.Orb != nil || pl.Gem != nil || pl.Python != nil
}

// registryOf returns the registry of an npm, orb, gem or Python plugin.
func registryOf(pl *spec.Plugin) registry {
	switch {
	case pl.NPM != nil:
		return newNPMRegistry(pl.NPM)
	case pl.Gem != nil:
		return newGemRegistry(pl.Gem)
	case pl.Python != nil:
		return newPythonRegistry(pl.Python)
	}
	return newOrbRegistry(pl.Orb)
}

// boxField returns a function that reads a field of the box config, as
// get_box_field in devbase shell/lib/box.sh does.
func boxField(field func(*box.Config) string) func() (string, error) {
	return func() (string, error) {
		c, err := box.LoadBox()
		if err != nil {
			return "", fmt.Errorf("read the box config: %w", err)
		}
		return field(c), nil
	}
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
	if err := p.rootSuccess(ctx); err != nil {
		return err
	}
	return p.unitSuccess(ctx)
}

// rootSuccess runs the success step of the root release.
func (p *publisher) rootSuccess(ctx context.Context) error {
	if !p.plan.RootRelease() {
		return nil
	}
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

// commits returns the commits of the root release, from the last
// release to the plan commit.
func (p *publisher) commits(ctx context.Context) ([]forge.Commit, error) {
	from := ""
	if p.plan.LastRelease != nil {
		from = p.plan.LastRelease.GitTag
	}
	raws, err := p.r.rootCommits(ctx, from, p.plan.Commit)
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
