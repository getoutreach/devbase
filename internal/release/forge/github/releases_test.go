// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of the GitHub releases and assets, ported from
// test/publish.test.js of @semantic-release/github 12.0.10.

package github

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// releasesPath is the API path of the releases of the test repository.
const releasesPath = "/repos/test_user/test_repo/releases"

// TestCreateRelease ports the publish.test.js cases that create a
// release without assets: the body of the POST, with prerelease and
// make_latest as the caller computes them from the branch.
func TestCreateRelease(t *testing.T) {
	tests := []struct {
		// name is the upstream test title.
		name string

		// prerelease and latest are the flags of the release.
		prerelease, latest bool
	}{
		{name: "Publish a release without creating discussion", latest: true},
		{name: "Publish a release on a channel", prerelease: true},
		{name: "Publish a prerelease without creating discussion", prerelease: true},
		{name: "Publish a maintenance release"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			releaseURL := "https://github.com/test_user/test_repo/releases/v1.0.0"
			f.expect(route{
				method: http.MethodPost,
				path:   releasesPath,
				body: map[string]any{
					"tag_name":         "v1.0.0",
					"target_commitish": "test_branch",
					"name":             "v1.0.0",
					"body":             "Test release note body",
					"prerelease":       tc.prerelease,
					"make_latest":      makeLatest(tc.latest),
				},
				response: map[string]any{"id": 1, "tag_name": "v1.0.0", "html_url": releaseURL},
			})
			var logs []string
			got, err := f.client(&logs).CreateRelease(context.Background(), &forge.ReleaseSpec{
				Tag:        "v1.0.0",
				Target:     "test_branch",
				Name:       "v1.0.0",
				Notes:      "Test release note body",
				Prerelease: tc.prerelease,
				Latest:     tc.latest,
			})
			assert.NilError(t, err)
			assert.DeepEqual(t, got, &forge.Release{ID: "1", Tag: "v1.0.0", URL: releaseURL})
		})
	}
}

// TestCreateReleaseGenerateNotes checks that GenerateNotes asks GitHub
// for the notes, as `gh release create unstable --prerelease=true
// --generate-notes` does in shell/ci/release/pre-release.sh, and leaves
// out the empty name and body.
func TestCreateReleaseGenerateNotes(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(route{
		method: http.MethodPost,
		path:   releasesPath,
		body: map[string]any{
			"tag_name":               "unstable",
			"target_commitish":       "main",
			"prerelease":             true,
			"make_latest":            "false",
			"draft":                  true,
			"generate_release_notes": true,
		},
		response: map[string]any{"id": 7, "tag_name": "unstable", "draft": true},
	})
	var logs []string
	got, err := f.client(&logs).CreateRelease(context.Background(), &forge.ReleaseSpec{
		Tag:           "unstable",
		Target:        "main",
		Prerelease:    true,
		Draft:         true,
		GenerateNotes: true,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, got, &forge.Release{ID: "7", Tag: "unstable", Draft: true})
}

// TestPublishReleaseWithAsset ports "Publish a release with one asset"
// of publish.test.js: a draft release, the upload of ".dotfile" to the
// upload_url of the draft, and the PATCH that publishes it. The asset
// label of that test is an option that our config does not have.
func TestPublishReleaseWithAsset(t *testing.T) {
	f := newFakeGitHub(t)
	uploadPath := "/api/uploads/repos/test_user/test_repo/releases/1/assets"
	releaseURL := "https://github.com/test_user/test_repo/releases/v1.0.0"
	assetURL := "https://github.com/test_user/test_repo/releases/download/v1.0.0/.dotfile"
	f.expect(
		route{
			method: http.MethodPost,
			path:   releasesPath,
			body: map[string]any{
				"tag_name":         "v1.0.0",
				"target_commitish": "test_branch",
				"name":             "v1.0.0",
				"body":             "Test release note body",
				"draft":            true,
				"prerelease":       false,
				"make_latest":      "true",
			},
			response: map[string]any{
				"id":         1,
				"tag_name":   "v1.0.0",
				"draft":      true,
				"upload_url": f.srv.URL + uploadPath + "{?name,label}",
				"html_url":   "https://github.com/test_user/test_repo/releases/untagged-123",
			},
		},
		route{
			method: http.MethodPost,
			path:   uploadPath,
			query:  url.Values{"name": {".dotfile"}},
			check: func(t *testing.T, r *http.Request, body []byte) {
				t.Helper()
				assert.Check(t, cmp.Equal(r.Header.Get("Content-Type"), "text/plain"))
				assert.Check(t, cmp.Equal(string(body), "Upload file content\n"))
			},
			response: map[string]any{
				"id": 5, "name": ".dotfile", "size": 20, "state": "uploaded", "browser_download_url": assetURL,
			},
		},
		route{
			method:   http.MethodPatch,
			path:     releasesPath + "/1",
			body:     map[string]any{"draft": false, "make_latest": "true"},
			response: map[string]any{"id": 1, "tag_name": "v1.0.0", "html_url": releaseURL},
		},
	)
	file := filepath.Join(t.TempDir(), ".dotfile")
	assert.NilError(t, os.WriteFile(file, []byte("Upload file content\n"), 0o600))

	var logs []string
	c := f.client(&logs)
	ctx := context.Background()
	draft, err := c.CreateRelease(ctx, &forge.ReleaseSpec{
		Tag: "v1.0.0", Target: "test_branch", Name: "v1.0.0", Notes: "Test release note body", Latest: true, Draft: true,
	})
	assert.NilError(t, err)
	asset, err := c.UploadAsset(ctx, draft, file)
	assert.NilError(t, err)
	assert.DeepEqual(t, asset, &forge.Asset{ID: "5", Name: ".dotfile", Size: 20, Complete: true})
	got, err := c.PublishRelease(ctx, draft, true)
	assert.NilError(t, err)
	assert.Equal(t, got.URL, releaseURL)
	assert.DeepEqual(t, logs, []string{"Published file " + assetURL, "Published GitHub release: " + releaseURL})
}

// TestUploadAssetContentType ports "Publish upload request does not set
// content-length manually" of publish.test.js: an asset without a known
// extension is "text/plain". The Content-Length header that upstream
// leaves to fetch is set by net/http from the file size, as fetch does
// on the wire. It also checks that the name is percent-encoded as in an
// Octokit URL template and that a release that this client did not
// create gets its upload_url from a GET.
func TestUploadAssetContentType(t *testing.T) {
	f := newFakeGitHub(t)
	uploadPath := "/repos/test_user/test_repo/releases/3/assets"
	f.expect(
		route{
			method: http.MethodGet,
			path:   releasesPath + "/3",
			response: map[string]any{
				"id": 3, "tag_name": "v1.0.0", "upload_url": f.srv.URL + uploadPath + "{?name,label}",
			},
		},
		route{
			method: http.MethodPost,
			path:   uploadPath,
			query:  url.Values{"name": {"my file+1.sha256"}},
			check: func(t *testing.T, r *http.Request, body []byte) {
				t.Helper()
				assert.Check(t, cmp.Equal(r.URL.RawQuery, "name=my%20file%2B1.sha256"))
				assert.Check(t, cmp.Equal(r.Header.Get("Content-Type"), "text/plain"))
				assert.Check(t, cmp.Equal(r.ContentLength, int64(len(body))))
			},
			response: map[string]any{"id": 9, "name": "my file+1.sha256", "size": 3, "state": "starter"},
		},
	)
	file := filepath.Join(t.TempDir(), "my file+1.sha256")
	assert.NilError(t, os.WriteFile(file, []byte("abc"), 0o600))
	var logs []string
	asset, err := f.client(&logs).UploadAsset(context.Background(), &forge.Release{ID: "3", Tag: "v1.0.0"}, file)
	assert.NilError(t, err)
	assert.DeepEqual(t, asset, &forge.Asset{ID: "9", Name: "my file+1.sha256", Size: 3})
}

// TestContentType checks the content types against the mime package of
// the plugin: mime.getType(extname(name)) || "text/plain".
func TestContentType(t *testing.T) {
	for name, want := range map[string]string{
		".dotfile":                 "text/plain",
		"upload.txt":               "text/plain",
		"devbase_1.0.0.tar.gz":     "application/gzip",
		"DEVBASE.TAR.GZ":           "application/gzip",
		"checksums.sha256":         "text/plain",
		"app.tgz":                  "text/plain",
		"app.zip":                  "application/zip",
		"sbom.spdx.json":           "application/json",
		"app.deb":                  "application/octet-stream",
		"app.rpm":                  "application/x-redhat-package-manager",
		"config.yaml":              "text/yaml",
		"checksums.txt.sig":        "application/pgp-signature",
		"cosign.pem":               "application/x-x509-ca-cert",
		"no_extension":             "text/plain",
		"trailing.":                "text/plain",
		"archive.tar.xz":           "application/x-xz",
		"multiple.dots.in.name.md": "text/markdown",
	} {
		assert.Check(t, cmp.Equal(contentType(name), want), name)
	}
}

// TestFindRelease checks that FindRelease lists the releases page by
// page to find a draft, and returns its assets.
func TestFindRelease(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(
		route{
			method:   http.MethodGet,
			path:     releasesPath,
			query:    url.Values{"per_page": {"100"}},
			header:   map[string]string{"Link": "<" + f.srv.URL + releasesPath + `?per_page=100&page=2>; rel="next"`},
			response: []map[string]any{{"id": 1, "tag_name": "v2.0.0"}},
		},
		route{
			method: http.MethodGet,
			path:   releasesPath,
			query:  url.Values{"per_page": {"100"}, "page": {"2"}},
			response: []map[string]any{{
				"id": 2, "tag_name": "v1.0.0", "draft": true, "html_url": "https://github.com/test_user/test_repo/releases/tag/v1.0.0",
				"assets": []map[string]any{
					{"id": 10, "name": "a.tar.gz", "size": 100, "state": "uploaded"},
					{"id": 11, "name": "b.tar.gz", "size": 50, "state": "starter"},
				},
			}},
		},
	)
	var logs []string
	got, err := f.client(&logs).FindRelease(context.Background(), "v1.0.0")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, &forge.Release{
		ID: "2", Tag: "v1.0.0", URL: "https://github.com/test_user/test_repo/releases/tag/v1.0.0", Draft: true,
		Assets: []forge.Asset{
			{ID: "10", Name: "a.tar.gz", Size: 100, Complete: true},
			{ID: "11", Name: "b.tar.gz", Size: 50},
		},
	})
}

// TestFindReleaseNone checks that FindRelease returns nil for a tag
// without a release.
func TestFindReleaseNone(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(route{
		method:   http.MethodGet,
		path:     releasesPath,
		query:    url.Values{"per_page": {"100"}},
		response: []map[string]any{{"id": 1, "tag_name": "v2.0.0"}},
	})
	var logs []string
	got, err := f.client(&logs).FindRelease(context.Background(), "v1.0.0")
	assert.NilError(t, err)
	assert.Assert(t, got == nil)
}

// TestDeleteAssetAndRelease checks the DELETE requests of an asset and
// a release.
func TestDeleteAssetAndRelease(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(
		route{method: http.MethodDelete, path: releasesPath + "/assets/11", status: http.StatusNoContent},
		route{method: http.MethodDelete, path: releasesPath + "/2", status: http.StatusNoContent},
	)
	var logs []string
	c := f.client(&logs)
	r := &forge.Release{ID: "2", Tag: "v1.0.0"}
	assert.NilError(t, c.DeleteAsset(context.Background(), r, &forge.Asset{ID: "11", Name: "b.tar.gz"}))
	assert.NilError(t, c.DeleteRelease(context.Background(), r))
}

// TestReleaseErrors checks that API failures wrap forge.ErrForge.
func TestReleaseErrors(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(route{
		method:   http.MethodPost,
		path:     releasesPath,
		status:   http.StatusUnprocessableEntity,
		response: map[string]any{"message": "Validation Failed"},
	})
	var logs []string
	_, err := f.client(&logs).CreateRelease(context.Background(), &forge.ReleaseSpec{Tag: "v1.0.0"})
	assert.Assert(t, errors.Is(err, forge.ErrForge), err)
	assert.Equal(t, statusCode(err), http.StatusUnprocessableEntity)

	_, err = New(Options{RepoURL: "https://github.com/o/r", BaseURL: "://bad"}).FindRelease(context.Background(), "v1.0.0")
	assert.ErrorContains(t, err, "parse GitHub API URL")
}
