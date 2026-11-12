// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: An in-memory forge for tests of the release flow.

// Package forgetest provides Fake, an in-memory forge.Forge for tests. It
// keeps releases and assets in memory, records Success and Fail calls,
// and can fail chosen calls, so that tests can check reruns and partial
// failures.
package forgetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// Fake is an in-memory forge. The zero value is not ready; use New.
type Fake struct {
	mu sync.Mutex

	// releases are the releases by tag.
	releases map[string]*forge.Release

	// nextID numbers releases and assets.
	nextID int

	// failures are errors to return from the next calls of a method, by
	// method name, in order.
	failures map[string][]error

	// Calls are the methods called, in order.
	Calls []string

	// Created are the arguments of the CreateRelease calls that made a
	// release.
	Created []forge.ReleaseSpec

	// Successes are the arguments of the Success calls.
	Successes []forge.Success

	// Failures are the arguments of the Fail calls, by branch.
	Failures map[string][][]error

	// Uploaded are the file contents of the assets, by tag and name.
	Uploaded map[string]map[string][]byte
}

// New returns an empty fake forge.
func New() *Fake {
	return &Fake{
		releases: map[string]*forge.Release{}, failures: map[string][]error{},
		Failures: map[string][][]error{}, Uploaded: map[string]map[string][]byte{},
	}
}

// FailNext makes the next call of method return err. Calls queue: a
// second FailNext fails the call after that.
func (f *Fake) FailNext(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[method] = append(f.failures[method], err)
}

// Release returns a copy of the release of tag, or nil.
func (f *Fake) Release(tag string) *forge.Release {
	f.mu.Lock()
	defer f.mu.Unlock()
	return clone(f.releases[tag])
}

// ReleaseName returns "Fake release".
func (f *Fake) ReleaseName() string {
	return "Fake release"
}

// FindRelease returns a copy of the release of tag, or nil.
func (f *Fake) FindRelease(_ context.Context, tag string) (*forge.Release, error) {
	if err := f.call("FindRelease"); err != nil {
		return nil, err
	}
	return f.Release(tag), nil
}

// CreateRelease adds a release. It fails when the tag has one.
func (f *Fake) CreateRelease(_ context.Context, spec *forge.ReleaseSpec) (*forge.Release, error) {
	if err := f.call("CreateRelease"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.releases[spec.Tag] != nil {
		return nil, fmt.Errorf("%w: release %s exists", forge.ErrForge, spec.Tag)
	}
	f.nextID++
	r := &forge.Release{
		ID: strconv.Itoa(f.nextID), Tag: spec.Tag, URL: "https://forge.test/releases/" + spec.Tag, Draft: spec.Draft,
	}
	f.releases[spec.Tag] = r
	f.Created = append(f.Created, *spec)
	return clone(r), nil
}

// UploadAsset reads path and adds it to the release as a complete asset.
func (f *Fake) UploadAsset(_ context.Context, r *forge.Release, path string) (*forge.Asset, error) {
	if err := f.call("UploadAsset"); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read asset: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stored := f.releases[r.Tag]
	if stored == nil {
		return nil, fmt.Errorf("%w: no release %s", forge.ErrForge, r.Tag)
	}
	name := filepath.Base(path)
	if slices.ContainsFunc(stored.Assets, func(a forge.Asset) bool { return a.Name == name }) {
		return nil, fmt.Errorf("%w: asset %s exists", forge.ErrForge, name)
	}
	f.nextID++
	a := forge.Asset{ID: strconv.Itoa(f.nextID), Name: name, Size: int64(len(b)), Complete: true}
	stored.Assets = append(stored.Assets, a)
	if f.Uploaded[r.Tag] == nil {
		f.Uploaded[r.Tag] = map[string][]byte{}
	}
	f.Uploaded[r.Tag][name] = b
	return &a, nil
}

// AddBrokenAsset adds an asset whose upload did not finish, as a crashed
// upload leaves it.
func (f *Fake) AddBrokenAsset(tag, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	r := f.releases[tag]
	r.Assets = append(r.Assets, forge.Asset{ID: strconv.Itoa(f.nextID), Name: name})
}

// DeleteAsset removes an asset from the release.
func (f *Fake) DeleteAsset(_ context.Context, r *forge.Release, a *forge.Asset) error {
	if err := f.call("DeleteAsset"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stored := f.releases[r.Tag]
	stored.Assets = slices.DeleteFunc(stored.Assets, func(x forge.Asset) bool { return x.ID == a.ID })
	delete(f.Uploaded[r.Tag], a.Name)
	return nil
}

// PublishRelease marks the release as not a draft.
func (f *Fake) PublishRelease(_ context.Context, r *forge.Release, _ bool) (*forge.Release, error) {
	if err := f.call("PublishRelease"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stored := f.releases[r.Tag]
	stored.Draft = false
	return clone(stored), nil
}

// DeleteRelease removes the release.
func (f *Fake) DeleteRelease(_ context.Context, r *forge.Release) error {
	if err := f.call("DeleteRelease"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.releases, r.Tag)
	delete(f.Uploaded, r.Tag)
	return nil
}

// Success records s.
func (f *Fake) Success(_ context.Context, s *forge.Success) error {
	if err := f.call("Success"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Successes = append(f.Successes, *s)
	return nil
}

// Fail records errs for branch.
func (f *Fake) Fail(_ context.Context, branch string, errs []error) error {
	if err := f.call("Fail"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Failures[branch] = append(f.Failures[branch], errs)
	return nil
}

// call records a call of method and returns its queued failure, if any.
func (f *Fake) call(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, method)
	queue := f.failures[method]
	if len(queue) == 0 {
		return nil
	}
	f.failures[method] = queue[1:]
	return queue[0]
}

// clone returns a deep copy of r, or nil.
func clone(r *forge.Release) *forge.Release {
	if r == nil {
		return nil
	}
	c := *r
	c.Assets = slices.Clone(r.Assets)
	return &c
}

// Compile-time check that Fake is a forge.
var _ forge.Forge = (*Fake)(nil)
