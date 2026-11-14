// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of the GitHub client options.

package github

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
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

// TestTokenTransportDebug checks that each request goes to the debug
// log, without the token.
func TestTokenTransportDebug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	var log bytes.Buffer
	ctx := rlog.WithDebug(context.Background(), &log)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/repos/o/r", http.NoBody)
	assert.NilError(t, err)
	resp, err := (&http.Client{Transport: &tokenTransport{token: "ghp_abc"}}).Do(req)
	assert.NilError(t, err)
	assert.NilError(t, resp.Body.Close())
	assert.Equal(t, log.String(), "debug: GET "+srv.URL+"/repos/o/r\n")
}

// TestVisibility checks that Visibility reads the visibility field of
// the repository, then the private field, and fails without both.
func TestVisibility(t *testing.T) {
	tests := []struct {
		// name describes the response.
		name string

		// response is the body of the repository GET.
		response map[string]any

		// want is the visibility; empty means an error.
		want string
	}{
		{name: "public", response: map[string]any{"visibility": "public", "private": false}, want: "public"},
		{name: "internal", response: map[string]any{"visibility": "internal", "private": true}, want: "internal"},
		{name: "private field only", response: map[string]any{"private": true}, want: "private"},
		{name: "public field only", response: map[string]any{"private": false}, want: "public"},
		{name: "no fields", response: map[string]any{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			f.expect(route{method: http.MethodGet, path: "/repos/test_user/test_repo", response: tc.response})
			var logs []string
			got, err := f.client(&logs).Visibility(context.Background())
			if tc.want == "" {
				assert.ErrorIs(t, err, errNoVisibility)
				assert.ErrorIs(t, err, forge.ErrForge)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}

	// An API error is a forge error.
	f := newFakeGitHub(t)
	f.expect(route{method: http.MethodGet, path: "/repos/test_user/test_repo", status: http.StatusNotFound})
	var logs []string
	_, err := f.client(&logs).Visibility(context.Background())
	assert.ErrorIs(t, err, forge.ErrForge)
}
