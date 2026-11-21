// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the GitHub version lister.

package stencilupgrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert"
)

func TestGitHubListerPagination(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/org/mod/releases", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Header.Get("Authorization"), "Bearer tok")
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"tag_name":"v1.2.0-rc.1","prerelease":true}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/repos/org/mod/releases?page=2>; rel="next"`, srv.URL))
		fmt.Fprint(w, `[{"tag_name":"v1.0.0","prerelease":false},{"tag_name":"v1.1.0","prerelease":false}]`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	vl := NewGitHubLister("tok", WithBaseURL(srv.URL+"/"))
	got, err := vl.Releases(context.Background(), "github.com/org/mod/v2/sub")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, []Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}, {Tag: "v1.2.0-rc.1", Prerelease: true}})
}

func TestGitHubListerDefaultBranch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/org/mod", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"default_branch":"trunk"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	vl := NewGitHubLister("", WithBaseURL(srv.URL+"/"))
	got, err := vl.DefaultBranch(context.Background(), "github.com/org/mod")
	assert.NilError(t, err)
	assert.Equal(t, got, "trunk")
}

func TestGitHubListerNonGitHubModule(t *testing.T) {
	vl := NewGitHubLister("")
	_, err := vl.Releases(context.Background(), "gitlab.com/org/mod")
	assert.ErrorContains(t, err, "gitlab.com")
	_, err = vl.DefaultBranch(context.Background(), "github.com/org")
	assert.Assert(t, err != nil)
}
