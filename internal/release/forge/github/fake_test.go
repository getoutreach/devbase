// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: A fake GitHub API for the tests, which checks each
// request as the nock and fetch-mock routes of the upstream tests do.

package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// testToken is the API token of the tests.
const testToken = "github_token"

// route is a request that the fake API expects, and its response.
type route struct {
	// method and path are the expected method and URL path.
	method, path string

	// query is the expected query; nil means no query.
	query url.Values

	// body, if not nil, is the expected JSON body.
	body any

	// graphql, if not empty, is the expected GraphQL operation name.
	graphql string

	// vars, with graphql, are the expected GraphQL variables.
	vars map[string]any

	// check, if not nil, checks the request and its body.
	check func(t *testing.T, r *http.Request, body []byte)

	// header are response headers.
	header map[string]string

	// status is the response status; 0 means 200.
	status int

	// response is the JSON response body; nil means "{}".
	response any
}

// fakeGitHub is a fake GitHub API that expects its routes in order.
type fakeGitHub struct {
	// t is the test.
	t *testing.T

	// srv is the HTTP server.
	srv *httptest.Server

	// mu guards routes and next.
	mu sync.Mutex

	// routes are the expected requests.
	routes []route

	// next is the index of the next expected request.
	next int
}

// newFakeGitHub starts a fake GitHub API, which fails the test at the
// end when an expected request did not come.
func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(func() {
		f.srv.Close()
		f.mu.Lock()
		defer f.mu.Unlock()
		assert.Check(t, cmp.Equal(f.next, len(f.routes)), "requests that did not come: %v", f.pending())
	})
	return f
}

// pending returns the method and path of the routes that did not come.
func (f *fakeGitHub) pending() []string {
	out := make([]string, 0, len(f.routes)-f.next)
	for _, r := range f.routes[f.next:] {
		out = append(out, r.method+" "+r.path)
	}
	return out
}

// expect adds expected requests.
func (f *fakeGitHub) expect(routes ...route) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes = append(f.routes, routes...)
}

// client returns a Client for the fake API and the repository
// test_user/test_repo, with the logs in *logs.
func (f *fakeGitHub) client(logs *[]string) *Client {
	return New(Options{
		Token:   testToken,
		RepoURL: "https://github.com/test_user/test_repo.git",
		BaseURL: f.srv.URL + "/",
		Log: func(format string, args ...any) {
			*logs = append(*logs, fmt.Sprintf(format, args...))
		},
	})
}

// serve checks a request against the next route and sends its response.
func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	t := f.t
	body, err := io.ReadAll(r.Body)
	assert.Check(t, err)
	f.mu.Lock()
	if f.next >= len(f.routes) {
		f.mu.Unlock()
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusTeapot)
		return
	}
	rt := f.routes[f.next]
	f.next++
	f.mu.Unlock()

	what := r.Method + " " + r.URL.String()
	assert.Check(t, cmp.Equal(r.Method, rt.method), what)
	assert.Check(t, cmp.Equal(r.URL.Path, rt.path), what)
	query := rt.query
	if query == nil {
		query = url.Values{}
	}
	assert.Check(t, cmp.DeepEqual(r.URL.Query(), query), what)
	assert.Check(t, cmp.Equal(r.Header.Get("Authorization"), "token "+testToken), what)
	if rt.body != nil {
		assert.Check(t, cmp.DeepEqual(decodeJSON(t, body), normalizeJSON(t, rt.body)), what)
	}
	if rt.graphql != "" {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		assert.Check(t, json.Unmarshal(body, &req), what)
		assert.Check(t, cmp.Contains(req.Query, "query "+rt.graphql+"("), what)
		assert.Check(t, cmp.DeepEqual(req.Variables, normalizeJSON(t, rt.vars)), what)
	}
	if rt.check != nil {
		rt.check(t, r, body)
	}

	w.Header().Set("Content-Type", "application/json")
	for k, v := range rt.header {
		w.Header().Set(k, v)
	}
	if rt.status != 0 {
		w.WriteHeader(rt.status)
	}
	if rt.status == http.StatusNoContent {
		return
	}
	resp := rt.response
	if resp == nil {
		resp = map[string]any{}
	}
	assert.Check(t, json.NewEncoder(w).Encode(resp))
}

// decodeJSON decodes a JSON body, or returns nil for an empty one.
func decodeJSON(t *testing.T, body []byte) any {
	t.Helper()
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	var v any
	assert.Check(t, json.Unmarshal(body, &v), string(body))
	return v
}

// normalizeJSON returns v as decoding its JSON gives it, so that it
// compares with a decoded body.
func normalizeJSON(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	assert.NilError(t, err)
	return decodeJSON(t, data)
}

// repoRoute is the GET of the repository that Success and Fail send
// first, which answers with fullName.
func repoRoute(fullName string) route {
	return route{
		method:   http.MethodGet,
		path:     "/repos/test_user/test_repo",
		response: map[string]any{"full_name": fullName},
	}
}

// srIssuesRoute is the getSRIssues query of find-sr-issues.js for
// owner/repo, which answers with issues.
func srIssuesRoute(owner, repo string, issues ...map[string]any) route {
	if issues == nil {
		issues = []map[string]any{}
	}
	return route{
		method:  http.MethodPost,
		path:    "/graphql",
		graphql: "getSRIssues",
		vars: map[string]any{
			"owner":  owner,
			"repo":   repo,
			"filter": map[string]any{"labels": []string{"semantic-release", "semantic-release"}},
		},
		response: map[string]any{"data": map[string]any{"repository": map[string]any{"issues": map[string]any{"nodes": issues}}}},
	}
}
