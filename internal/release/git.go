// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Reads tags, git notes and commits with the git CLI.

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// These are the separators in the git log format that Commits uses.
const (
	// fieldSep separates the hash from the message in one record.
	fieldSep = "\x1f"

	// recordSep separates the commits.
	recordSep = "\x1e"
)

// Git runs git commands in a repository.
type Git struct {
	// Dir is the repository directory.
	Dir string
}

// ErrUnsupportedTag is returned for a tag that semantic-release accepts as
// a release tag but cannot compare, such as "v=1.0.0".
var ErrUnsupportedTag = errors.New("unsupported release tag")

// Tag is a release tag with the channels from its git note.
type Tag struct {
	Name     string
	Version  *semver.Version
	Channels []string
}

// RawCommit is a commit as git log reports it.
type RawCommit struct {
	Hash    string
	Message string
}

// Run runs git with args and returns its trimmed stdout.
func (g *Git) Run(ctx context.Context, args ...string) (string, error) {
	return run(ctx, g.Dir, nil, "git", args...)
}

// RemoteURL returns the configured URL of the origin remote, not the one
// after url.<base>.insteadOf rewrites, as semantic-release reads it.
func (g *Git) RemoteURL(ctx context.Context) (string, error) {
	return g.Run(ctx, "config", "--get", "remote.origin.url")
}

// RepoURL returns the https URL of the origin remote.
func (g *Git) RepoURL(ctx context.Context) (string, error) {
	remote, err := g.RemoteURL(ctx)
	if err != nil {
		return "", err
	}
	return RepoURLFromRemote(remote)
}

// Fetch fetches all tags and the branch from the remote at url, as
// semantic-release does before it plans. current is the checked-out
// branch, or "HEAD" when HEAD is detached; when it is not branch, the
// fetch updates refs/heads/<branch> with an explicit refspec.
func (g *Git) Fetch(ctx context.Context, url, branch, current string) error {
	args := []string{"--tags", "--", url}
	if current != branch {
		args = []string{"--tags", "--update-head-ok", "--", url, "+refs/heads/" + branch + ":refs/heads/" + branch}
	}
	return g.fetch(ctx, args...)
}

// FetchNotes fetches every git notes ref from the remote at url, which
// hold the channels of the release tags. As in semantic-release, a
// failure is ignored: a remote without notes has no refs to fetch.
func (g *Git) FetchNotes(ctx context.Context, url string) {
	//nolint:errcheck // Why: semantic-release ignores this failure too.
	g.fetch(ctx, "--", url, "+refs/notes/*:refs/notes/*")
}

// CurrentBranch returns the checked-out branch, or "HEAD" when HEAD is
// detached.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	return g.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// RemoteHead returns the commit of the branch on the remote at url, or
// "" when the remote has no such branch.
func (g *Git) RemoteHead(ctx context.Context, url, branch string) (string, error) {
	out, err := g.Run(ctx, "ls-remote", "--heads", "--", url, branch)
	head, _, _ := strings.Cut(out, "\t")
	return head, err
}

// IsAncestor reports whether commit a is an ancestor of commit b, or b.
func (g *Git) IsAncestor(ctx context.Context, a, b string) bool {
	_, err := g.Run(ctx, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// Tags returns the release tags (v<semver>) that are merged into ref, or
// all of them when ref is "", with their channels. A tag without a git note is on the default
// channel, which is the empty string.
func (g *Git) Tags(ctx context.Context, ref string) ([]Tag, error) {
	args := []string{"tag", "--list", "v*"}
	if ref != "" {
		args = append(args, "--merged", ref)
	}
	out, err := g.Run(ctx, args...)
	if err != nil || out == "" {
		return nil, err
	}
	channels, err := g.tagChannels(ctx)
	if err != nil {
		return nil, err
	}
	var tags []Tag
	for name := range strings.FieldsSeq(out) {
		// The tag format is v${version}, so the version follows the "v".
		version := strings.TrimPrefix(name, "v")
		v, err := ParseVersion(version)
		if err != nil {
			if isCleanVersion(version) {
				// semantic-release keeps this tag, then fails when it
				// compares the version.
				return nil, fmt.Errorf("%w: %q", ErrUnsupportedTag, name)
			}
			continue // Not a release tag.
		}
		ch, ok := channels[name]
		if !ok {
			ch = []string{""}
		}
		tags = append(tags, Tag{Name: name, Version: v, Channels: ch})
	}
	return tags, nil
}

// Commits returns the commits in from..to, newest first. An empty from
// means all commits reachable from to.
func (g *Git) Commits(ctx context.Context, from, to string) ([]RawCommit, error) {
	rng := to
	if from != "" {
		rng = from + ".." + to
	}
	out, err := g.Run(ctx, "log", "--format=%H"+fieldSep+"%B"+recordSep, rng)
	if err != nil {
		return nil, err
	}
	var commits []RawCommit
	for rec := range strings.SplitSeq(out, recordSep) {
		hash, msg, ok := strings.Cut(strings.TrimSpace(rec), fieldSep)
		if ok {
			commits = append(commits, RawCommit{Hash: hash, Message: jsTrim(msg)})
		}
	}
	return commits, nil
}

// fetch runs git fetch with --unshallow, and again without it when that
// fails, as it does on a complete repository.
func (g *Git) fetch(ctx context.Context, args ...string) error {
	if _, err := g.Run(ctx, append([]string{"fetch", "--unshallow"}, args...)...); err == nil {
		return nil
	}
	_, err := g.Run(ctx, append([]string{"fetch"}, args...)...)
	return err
}

// tagChannels reads the channels of every tag from the semantic-release
// git notes, with the same git log call that semantic-release uses. When
// two tags share a commit, both get the first note, as in
// semantic-release.
func (g *Git) tagChannels(ctx context.Context) (map[string][]string, error) {
	out, err := g.Run(ctx, "log", "--tags=*", "--decorate-refs=refs/tags/*", "--no-walk",
		"--format=%d%x09%N", "--notes=refs/notes/semantic-release*")
	if err != nil {
		return nil, err
	}
	channels := map[string][]string{}
	for line := range strings.SplitSeq(out, "\n") {
		deco, note, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		var n struct {
			Channels []*string `json:"channels"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(note)), &n) != nil {
			continue
		}
		chs := make([]string, len(n.Channels))
		for i, c := range n.Channels {
			if c != nil {
				chs[i] = *c
			}
		}
		for d := range strings.SplitSeq(strings.Trim(strings.TrimSpace(deco), "()"), ",") {
			if name, ok := strings.CutPrefix(strings.TrimSpace(d), "tag: "); ok {
				channels[name] = chs
			}
		}
	}
	return channels, nil
}

// run runs a command in dir and returns its trimmed stdout. A nil env
// means the current environment. On failure, the error has stderr.
func run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()+"\n"+stdout.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
