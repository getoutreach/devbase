// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The Git forge interface: what a release needs from GitHub
// or another forge.

// Package forge defines the interface between the release command and a
// Git forge, such as GitHub. The version engine, the notes renderer, the
// builder and the publish flow use only this interface and plain git, so
// another forge implementation can replace GitHub without changes to them.
package forge

import (
	"context"
	"errors"
)

// ErrForge is returned for a forge API call that fails.
var ErrForge = errors.New("forge request failed")

// Public is the visibility of a repository that everyone can read.
const Public = "public"

// Forge is a Git forge. Tags and git notes go through the git CLI, not
// through a forge.
type Forge interface {
	// ReleaseName names a forge release in success comments, such as
	// "GitHub release".
	ReleaseName() string

	// FindRelease returns the release of a tag, including a draft, or nil
	// when there is none.
	FindRelease(ctx context.Context, tag string) (*Release, error)

	// CreateRelease creates a release. With assets to upload, the caller
	// creates a draft, uploads the assets, and then publishes it.
	CreateRelease(ctx context.Context, spec *ReleaseSpec) (*Release, error)

	// UploadAsset uploads a file to a release under its base name.
	UploadAsset(ctx context.Context, r *Release, path string) (*Asset, error)

	// DeleteAsset deletes an asset of a release, such as a partial upload.
	DeleteAsset(ctx context.Context, r *Release, a *Asset) error

	// PublishRelease turns a draft into a published release. latest
	// marks it as the latest release of the repository.
	PublishRelease(ctx context.Context, r *Release, latest bool) (*Release, error)

	// DeleteRelease deletes a release, but not its tag.
	DeleteRelease(ctx context.Context, r *Release) error

	// Success tells the forge that a release is out: it comments on and
	// labels the change requests and issues that the release resolves,
	// with the forge's own rules to find them, and closes the open
	// failure issues.
	Success(ctx context.Context, s *Success) error

	// Fail opens or updates the failure issue of a branch with errs.
	Fail(ctx context.Context, branch string, errs []error) error

	// Visibility returns the visibility of the repository: Public, or
	// another value of the forge, such as "private" or "internal".
	Visibility(ctx context.Context) (string, error)
}

// NoteLinks builds the links in release notes.
type NoteLinks interface {
	// Compare returns the URL that compares two tags.
	Compare(from, to string) string

	// Commit returns the URL of a commit.
	Commit(hash string) string

	// Issue returns the URL of an issue. An empty owner or repo means
	// the owner or repository of the release.
	Issue(owner, repo, id string) string

	// User returns the URL of a user.
	User(name string) string
}

// ReleaseSpec describes a release to create.
type ReleaseSpec struct {
	// Tag is the git tag, such as "v1.2.3".
	Tag string

	// Target is the branch that the release comes from.
	Target string

	// Name is the release title.
	Name string

	// Notes is the release body, in Markdown.
	Notes string

	// Prerelease marks a release that is not for production.
	Prerelease bool

	// Latest marks the latest release of the repository.
	Latest bool

	// Draft creates a release that is not visible yet.
	Draft bool

	// GenerateNotes asks the forge to write the notes, instead of Notes,
	// as the unstable release does.
	GenerateNotes bool
}

// Release is a release on the forge.
type Release struct {
	// ID identifies the release on the forge.
	ID string

	// Tag is the git tag of the release.
	Tag string

	// URL is the web page of the release.
	URL string

	// Draft is true for a release that is not published yet.
	Draft bool

	// Assets are the uploaded files, so that a rerun uploads only the
	// files that are missing or broken.
	Assets []Asset
}

// Asset is a file of a release.
type Asset struct {
	// ID identifies the asset on the forge.
	ID string

	// Name is the file name.
	Name string

	// Size is the size in bytes.
	Size int64

	// Complete is false for an upload that did not finish.
	Complete bool
}

// Success describes a release that is out, for Forge.Success.
type Success struct {
	// Unit names the released unit, such as an npm package name, or is
	// empty for the root release. The comments name it.
	Unit string

	// Version is the released version, such as "1.2.3".
	Version string

	// Channel is the release channel, or empty for the default channel.
	Channel string

	// Commits are the commits of the release.
	Commits []Commit

	// Releases are the places where the release is available, in
	// publish order, such as the forge release and a package registry.
	Releases []Published
}

// Commit is a commit of a release.
type Commit struct {
	Hash    string
	Message string
}

// Published is a place where a release is available.
type Published struct {
	// Name is the name of the place, such as "GitHub release".
	Name string `json:"name"`

	// URL is the address of the release there, or empty.
	URL string `json:"url"`
}
