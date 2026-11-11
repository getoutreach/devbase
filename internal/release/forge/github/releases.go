// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: GitHub releases and release assets, as a port of
// publish.js of @semantic-release/github.

package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gh "github.com/google/go-github/v84/github"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// listPerPage is the page size of the release list.
const listPerPage = 100

// FindRelease returns the release of tag, including a draft, or nil when
// there is none. It lists the releases, because GET releases/tags/{tag}
// does not return drafts.
func (c *Client) FindRelease(ctx context.Context, tag string) (*forge.Release, error) {
	if c.err != nil {
		return nil, c.err
	}
	opts := &gh.ListOptions{PerPage: listPerPage}
	for {
		releases, resp, err := c.gh.Repositories.ListReleases(ctx, c.owner, c.repo, opts)
		if err != nil {
			return nil, apiError("list releases", err)
		}
		for _, r := range releases {
			if r.GetTagName() == tag {
				return c.toRelease(r), nil
			}
		}
		if resp.NextPage == 0 {
			return nil, nil //nolint:nilnil // Why: the interface returns nil for no release.
		}
		opts.Page = resp.NextPage
	}
}

// CreateRelease creates a release with the fields of publish.js:
// tag_name, target_commitish, name, body, prerelease and make_latest,
// and draft only for a draft. With GenerateNotes, GitHub writes the
// notes, and the name and body are sent only when they are not empty.
func (c *Client) CreateRelease(ctx context.Context, spec *forge.ReleaseSpec) (*forge.Release, error) {
	if c.err != nil {
		return nil, c.err
	}
	req := &gh.RepositoryRelease{
		TagName:         gh.Ptr(spec.Tag),
		TargetCommitish: gh.Ptr(spec.Target),
		Name:            gh.Ptr(spec.Name),
		Body:            gh.Ptr(spec.Notes),
		Prerelease:      gh.Ptr(spec.Prerelease),
		MakeLatest:      gh.Ptr(makeLatest(spec.Latest)),
	}
	if spec.Draft {
		req.Draft = gh.Ptr(true)
	}
	if spec.GenerateNotes {
		req.GenerateReleaseNotes = gh.Ptr(true)
		if spec.Name == "" {
			req.Name = nil
		}
		if spec.Notes == "" {
			req.Body = nil
		}
	}
	r, _, err := c.gh.Repositories.CreateRelease(ctx, c.owner, c.repo, req)
	if err != nil {
		return nil, apiError("create release "+spec.Tag, err)
	}
	return c.toRelease(r), nil
}

// UploadAsset uploads the file at path to the upload_url of the release
// under its base name, with the content type of its extension.
func (c *Client) UploadAsset(ctx context.Context, r *forge.Release, path string) (*forge.Asset, error) {
	if c.err != nil {
		return nil, c.err
	}
	uploadURL, err := c.uploadURL(ctx, r)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open asset: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat asset: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("asset %s: %w", path, errNotAFile)
	}

	// The upload_url is a URI template such as ".../assets{?name,label}".
	name := filepath.Base(path)
	target, _, _ := strings.Cut(uploadURL, "{")
	target += "?name=" + encodeUnreserved(name)
	req, err := c.gh.NewUploadRequest(target, f, info.Size(), contentType(name))
	if err != nil {
		return nil, fmt.Errorf("build upload request: %w", err)
	}
	var asset gh.ReleaseAsset
	if _, err := c.gh.Do(ctx, req, &asset); err != nil {
		return nil, apiError("upload asset "+name, err)
	}
	c.logf("Published file %s", asset.GetBrowserDownloadURL())
	a := toAsset(&asset)
	return &a, nil
}

// errNotAFile is returned for an asset path that is not a regular file.
var errNotAFile = errors.New("not a file")

// DeleteAsset deletes an asset of a release.
func (c *Client) DeleteAsset(ctx context.Context, _ *forge.Release, a *forge.Asset) error {
	if c.err != nil {
		return c.err
	}
	id, err := parseID(a.ID)
	if err != nil {
		return err
	}
	if _, err := c.gh.Repositories.DeleteReleaseAsset(ctx, c.owner, c.repo, id); err != nil {
		return apiError("delete asset "+a.Name, err)
	}
	return nil
}

// PublishRelease turns a draft into a published release with the PATCH
// of publish.js: draft false and make_latest.
func (c *Client) PublishRelease(ctx context.Context, r *forge.Release, latest bool) (*forge.Release, error) {
	if c.err != nil {
		return nil, c.err
	}
	id, err := parseID(r.ID)
	if err != nil {
		return nil, err
	}
	req := &gh.RepositoryRelease{Draft: gh.Ptr(false), MakeLatest: gh.Ptr(makeLatest(latest))}
	out, _, err := c.gh.Repositories.EditRelease(ctx, c.owner, c.repo, id, req)
	if err != nil {
		return nil, apiError("publish release "+r.Tag, err)
	}
	c.logf("Published GitHub release: %s", out.GetHTMLURL())
	return c.toRelease(out), nil
}

// DeleteRelease deletes a release, but not its tag.
func (c *Client) DeleteRelease(ctx context.Context, r *forge.Release) error {
	if c.err != nil {
		return c.err
	}
	id, err := parseID(r.ID)
	if err != nil {
		return err
	}
	if _, err := c.gh.Repositories.DeleteRelease(ctx, c.owner, c.repo, id); err != nil {
		return apiError("delete release "+r.Tag, err)
	}
	return nil
}

// uploadURL returns the upload_url of a release: the one that an API
// response gave for it, or else the one of a GET of the release.
func (c *Client) uploadURL(ctx context.Context, r *forge.Release) (string, error) {
	c.mu.Lock()
	u, ok := c.uploadURLs[r.ID]
	c.mu.Unlock()
	if ok {
		return u, nil
	}
	id, err := parseID(r.ID)
	if err != nil {
		return "", err
	}
	got, _, err := c.gh.Repositories.GetRelease(ctx, c.owner, c.repo, id)
	if err != nil {
		return "", apiError("get release "+r.Tag, err)
	}
	if u = got.GetUploadURL(); u == "" {
		return "", fmt.Errorf("release %s: %w", r.Tag, errNoUploadURL)
	}
	return u, nil
}

// errNoUploadURL is returned for a release without an upload_url.
var errNoUploadURL = errors.New("release has no upload_url")

// toRelease converts a GitHub release and keeps its upload_url.
func (c *Client) toRelease(r *gh.RepositoryRelease) *forge.Release {
	out := &forge.Release{
		ID:    strconv.FormatInt(r.GetID(), 10),
		Tag:   r.GetTagName(),
		URL:   r.GetHTMLURL(),
		Draft: r.GetDraft(),
	}
	for _, a := range r.Assets {
		out.Assets = append(out.Assets, toAsset(a))
	}
	if r.UploadURL != nil {
		c.mu.Lock()
		c.uploadURLs[out.ID] = r.GetUploadURL()
		c.mu.Unlock()
	}
	return out
}

// toAsset converts a GitHub release asset. Only an asset in the
// "uploaded" state is complete.
func toAsset(a *gh.ReleaseAsset) forge.Asset {
	return forge.Asset{
		ID:       strconv.FormatInt(a.GetID(), 10),
		Name:     a.GetName(),
		Size:     int64(a.GetSize()),
		Complete: a.GetState() == "uploaded",
	}
}

// parseID parses a decimal GitHub ID.
func parseID(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse GitHub ID %q: %w", id, err)
	}
	return n, nil
}

// makeLatest returns the make_latest value of is-latest-release.js.
func makeLatest(latest bool) string {
	return strconv.FormatBool(latest)
}

// mimeTypes maps file extensions, in lowercase and without the dot, to
// the content type that the mime package (version 4) gives for them.
// It covers the files that releases usually have; mime.getType returns
// null for "sha256", "tgz", "zst" and "whl", for example, so they get
// "text/plain" as other unknown extensions do.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var mimeTypes = map[string]string{
	"7z":   "application/x-7z-compressed",
	"apk":  "application/vnd.android.package-archive",
	"asc":  "application/pgp-keys",
	"bin":  "application/octet-stream",
	"bz":   "application/x-bzip",
	"bz2":  "application/x-bzip2",
	"cdx":  "chemical/x-cdx",
	"cer":  "application/pkix-cert",
	"conf": "text/plain",
	"crt":  "application/x-x509-ca-cert",
	"crx":  "application/x-chrome-extension",
	"csv":  "text/csv",
	"deb":  "application/octet-stream",
	"dll":  "application/octet-stream",
	"dmg":  "application/octet-stream",
	"ear":  "application/java-archive",
	"exe":  "application/octet-stream",
	"gz":   "application/gzip",
	"html": "text/html",
	"img":  "application/octet-stream",
	"ini":  "text/plain",
	"iso":  "application/octet-stream",
	"jar":  "application/java-archive",
	"js":   "text/javascript",
	"json": "application/json",
	"log":  "text/plain",
	"md":   "text/markdown",
	"msi":  "application/octet-stream",
	"pdf":  "application/pdf",
	"pem":  "application/x-x509-ca-cert",
	"pkg":  "application/octet-stream",
	"png":  "image/png",
	"pub":  "application/x-mspublisher",
	"rpm":  "application/x-redhat-package-manager",
	"sh":   "application/x-sh",
	"sig":  "application/pgp-signature",
	"so":   "application/octet-stream",
	"spdx": "text/spdx",
	"svg":  "image/svg+xml",
	"tar":  "application/x-tar",
	"toml": "application/toml",
	"txt":  "text/plain",
	"war":  "application/java-archive",
	"wasm": "application/wasm",
	"xml":  "application/xml",
	"xpi":  "application/x-xpinstall",
	"xz":   "application/x-xz",
	"yaml": "text/yaml",
	"yml":  "text/yaml",
	"zip":  "application/zip",
}

// contentType returns the content type of a file name, as
// `mime.getType(extname(fileName)) || "text/plain"` does in publish.js.
func contentType(name string) string {
	ext := filepath.Ext(name)
	// extname gives no extension for a name that starts with its only
	// dot, such as ".dotfile".
	if ext == name {
		ext = ""
	}
	if t, ok := mimeTypes[strings.ToLower(strings.TrimPrefix(ext, "."))]; ok {
		return t
	}
	return "text/plain"
}
