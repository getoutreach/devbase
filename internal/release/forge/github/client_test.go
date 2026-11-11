// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of the GitHub client options.

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert"
)

// TestReleaseNameAndRepo checks the release name of
// definitions/constants.js and the repository from the URL.
func TestReleaseNameAndRepo(t *testing.T) {
	c := New(Options{RepoURL: "git@github.example.com:o/r.git"})
	assert.Equal(t, c.ReleaseName(), "GitHub release")
	assert.Equal(t, c.owner+"/"+c.repo, "o/r")

	// A URL without an owner and a name makes the API calls fail.
	_, err := New(Options{RepoURL: "https://github.com/o"}).FindRelease(context.Background(), "v1.0.0")
	assert.ErrorIs(t, err, errRepoURL)
}

// TestTokenTransport checks the Authorization header of
// @octokit/auth-token: "token" for a token, "bearer" for a JWT, and no
// header without a token.
func TestTokenTransport(t *testing.T) {
	for token, want := range map[string]string{
		"ghp_abc":       "token ghp_abc",
		"Token ghp_abc": "token ghp_abc",
		"a.b.c":         "bearer a.b.c",
		"bearer a.b.c":  "bearer a.b.c",
		"":              "",
	} {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("Authorization")
		}))
		hc := &http.Client{Transport: &tokenTransport{token: token}}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
		assert.NilError(t, err)
		resp, err := hc.Do(req)
		assert.NilError(t, err)
		assert.NilError(t, resp.Body.Close())
		srv.Close()
		assert.Equal(t, got, want, token)
	}
}
