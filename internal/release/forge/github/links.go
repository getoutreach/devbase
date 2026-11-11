// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: github.com style links in release notes.

package github

import (
	"cmp"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// links builds github.com style links for a repository.
type links struct {
	// Host is the scheme and host, such as "https://github.com".
	Host string

	// Owner and Repo name the repository.
	Owner, Repo string
}

// NewLinks returns the links for an https repository URL without
// ".git", as RepoURLFromRemote returns it. As in
// release-notes-generator, the first path element is the owner and the
// rest is the repository.
func NewLinks(repoURL string) forge.NoteLinks {
	scheme, rest, _ := strings.Cut(repoURL, "://")
	host, path, _ := strings.Cut(rest, "/")
	owner, repo, _ := strings.Cut(path, "/")
	return links{Host: scheme + "://" + host, Owner: owner, Repo: repo}
}

// Compare returns the compare URL between two tags.
func (l links) Compare(from, to string) string {
	return l.repoURL() + "/compare/" + from + "..." + to
}

// Commit returns the URL of a commit.
func (l links) Commit(hash string) string {
	return l.repoURL() + "/commit/" + hash
}

// Issue returns the URL of an issue. An empty owner or repo means the
// owner or repository of l.
func (l links) Issue(owner, repo, id string) string {
	return l.Host + "/" + cmp.Or(owner, l.Owner) + "/" + cmp.Or(repo, l.Repo) + "/issues/" + id
}

// User returns the URL of a user.
func (l links) User(name string) string {
	return l.Host + "/" + name
}

// repoURL returns the URL of the repository.
func (l links) repoURL() string {
	return l.Host + "/" + l.Owner + "/" + l.Repo
}
