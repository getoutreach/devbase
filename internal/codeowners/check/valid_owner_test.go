// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the check that validates the owners of CODEOWNERS entries.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v84/github"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// Routes of the fake GitHub REST API, for the repository "org/repo".
const (
	routeRepository    = "GET /repos/org/repo"
	routeCollaborators = "GET /repos/org/repo/collaborators"
	routeTeams         = "GET /repos/org/repo/teams"
	routeTeamRepo      = "GET /orgs/org/teams/{slug}/repos/org/repo"
)

// Responses of the fake GitHub REST API.
const (
	collaboratorsJSON = `[
		{"login": "alice", "permissions": {"admin": false, "maintain": false, "push": true, "triage": true, "pull": true}},
		{"login": "bob", "permissions": {"admin": false, "maintain": false, "push": false, "triage": true, "pull": true}},
		{"login": "carol", "permissions": {"maintain": true}},
		{"login": "dave", "permissions": {"admin": true}}
	]`
	teamsJSON = `[
		{"slug": "writers", "permissions": {"admin": false, "maintain": false, "push": true, "triage": true, "pull": true}},
		{"slug": "readers", "permissions": {"admin": false, "maintain": false, "push": false, "triage": true, "pull": true}},
		{"slug": "maintainers", "permissions": {"maintain": true}},
		{"slug": "push-string", "permission": "push"},
		{"slug": "pull-string", "permission": "pull"}
	]`
	notFoundJSON = `{"message": "Not Found"}`
)

// fakeGitHub is a fake GitHub REST API that counts the requests per route.
type fakeGitHub struct {
	// mu guards requests.
	mu sync.Mutex

	// requests is the number of requests per route.
	requests map[string]int
}

// newFakeGitHub starts a fake GitHub REST API that serves routes, and returns a client for it.
// A route is a pattern that http.ServeMux accepts.
func newFakeGitHub(t *testing.T, routes map[string]http.HandlerFunc) (*github.Client, *fakeGitHub) {
	t.Helper()

	f := &fakeGitHub{requests: map[string]int{}}
	mux := http.NewServeMux()
	for route, h := range routes {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			f.requests[route]++
			f.mu.Unlock()

			h(w, r)
		})
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	baseURL, err := url.Parse(srv.URL + "/")
	assert.NilError(t, err)

	gh := github.NewClient(nil)
	gh.BaseURL = baseURL

	return gh, f
}

// count returns the number of requests to route.
func (f *fakeGitHub) count(route string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.requests[route]
}

// reply returns a handler that responds with status and body, and with the X-OAuth-Scopes header
// when scopes is not empty.
func reply(status int, scopes, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if scopes != "" {
			w.Header().Set("X-Oauth-Scopes", scopes)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// teamRepoReply returns a handler for routeTeamRepo that responds with the body for the team slug,
// or with 404 for a team that is not in bodies.
func teamRepoReply(bodies map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.PathValue("slug")]
		if !ok {
			reply(http.StatusNotFound, "", notFoundJSON)(w, r)
			return
		}
		reply(http.StatusOK, "", body)(w, r)
	}
}

// defaultRoutes returns the routes of a fake GitHub REST API where all calls succeed.
func defaultRoutes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		routeRepository:    reply(http.StatusOK, "", `{"name": "repo"}`),
		routeCollaborators: reply(http.StatusOK, "", collaboratorsJSON),
		routeTeams:         reply(http.StatusOK, "", teamsJSON),
		routeTeamRepo: teamRepoReply(map[string]string{
			"child":        `{"name": "repo", "permissions": {"admin": false, "push": true, "pull": true}}`,
			"child-reader": `{"name": "repo", "permissions": {"admin": false, "push": false, "pull": true}}`,
		}),
	}
}

// newTestValidOwner returns a ValidOwner for the repository "org/repo" that uses gh.
func newTestValidOwner(t *testing.T, gh *github.Client) *ValidOwner {
	t.Helper()

	v, err := NewValidOwner(ValidOwnerConfig{Repository: "org/repo"}, gh)
	assert.NilError(t, err)

	return v
}

// isValidOwner reports whether owner has the syntax of an owner.
func isValidOwner(owner string) bool {
	return isEmailAddress(owner) || isGitHubUser(owner) || isGitHubTeam(owner)
}

func TestValidOwnerChecker(t *testing.T) {
	tests := map[string]struct {
		owner   string
		isValid bool
	}{
		"Invalid Email": {owner: `asda.comm`, isValid: false},
		"Valid Email":   {owner: `gmail@gmail.com`, isValid: true},
		"Invalid Team":  {owner: `@org/`, isValid: false},
		"Valid Team":    {owner: `@org/user`, isValid: true},
		"Invalid User":  {owner: `user`, isValid: false},
		"Valid User":    {owner: `@user`, isValid: true},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, isValidOwner(tc.owner), tc.isValid)
		})
	}
}

func TestNewValidOwnerInvalidRepository(t *testing.T) {
	for _, repo := range []string{"", "org", "org/", "/repo", "org/repo/x"} {
		t.Run(repo, func(t *testing.T) {
			_, err := NewValidOwner(ValidOwnerConfig{Repository: repo}, nil)
			assert.Assert(t, errors.Is(err, ErrInvalidRepository), "got %v", err)
		})
	}
}

func TestValidOwnerCheckerIgnoredOwner(t *testing.T) {
	t.Run("Should ignore owner", func(t *testing.T) {
		ownerCheck, err := NewValidOwner(ValidOwnerConfig{
			Repository:    "org/repo",
			IgnoredOwners: []string{"@owner1"},
		}, nil)
		assert.NilError(t, err)

		out, err := ownerCheck.Check(t.Context(), loadInput(t, `*	@owner1`))

		assert.NilError(t, err)
		assert.Equal(t, len(out.Issues), 0)
	})

	t.Run("Should ignore user only and check the remaining owners", func(t *testing.T) {
		tests := map[string]struct {
			codeowners           string
			issue                *Issue
			allowUnownedPatterns bool
		}{
			"No owners": {
				codeowners: `*`,
				issue: &Issue{
					Severity: Warning,
					LineNo:   1,
					Message:  "Missing owner, at least one owner is required",
				},
			},
			"Bad owner definition": {
				codeowners: `*	badOwner @owner1`,
				issue: &Issue{
					Severity: Error,
					LineNo:   1,
					Message:  `Not valid owner definition "badOwner"`,
				},
			},
			"No owners but allow empty": {
				codeowners:           `*`,
				issue:                nil,
				allowUnownedPatterns: true,
			},
		}
		for tn, tc := range tests {
			t.Run(tn, func(t *testing.T) {
				ownerCheck, err := NewValidOwner(ValidOwnerConfig{
					Repository:           "org/repo",
					AllowUnownedPatterns: tc.allowUnownedPatterns,
					IgnoredOwners:        []string{"@owner1"},
				}, nil)
				assert.NilError(t, err)

				out, err := ownerCheck.Check(t.Context(), loadInput(t, tc.codeowners))

				assert.NilError(t, err)
				assertSingleIssue(t, tc.issue, out.Issues)
			})
		}
	})
}

func TestValidOwnerCheckerOwnersMustBeTeams(t *testing.T) {
	tests := map[string]struct {
		codeowners           string
		issue                *Issue
		allowUnownedPatterns bool
	}{
		"Bad owner definition": {
			codeowners: `*	@owner1`,
			issue: &Issue{
				Severity: Error,
				LineNo:   1,
				Message:  `Only team owners allowed and "@owner1" is not a team`,
			},
		},
		"Email owner": {
			codeowners: `*	owner@example.com`,
			issue: &Issue{
				Severity: Error,
				LineNo:   1,
				Message:  `Only team owners allowed and "owner@example.com" is not a team`,
			},
		},
		"Team of another organization": {
			codeowners: `*	@other/team`,
			issue: &Issue{
				Severity: Error,
				LineNo:   1,
				Message:  `Team "@other/team" does not belong to "org" organization.`,
			},
		},
		"No owners but allow empty": {
			codeowners:           `*`,
			issue:                nil,
			allowUnownedPatterns: true,
		},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			ownerCheck, err := NewValidOwner(ValidOwnerConfig{
				Repository:           "org/repo",
				AllowUnownedPatterns: tc.allowUnownedPatterns,
				OwnersMustBeTeams:    true,
			}, nil)
			assert.NilError(t, err)

			out, err := ownerCheck.Check(t.Context(), loadInput(t, tc.codeowners))

			assert.NilError(t, err)
			assertSingleIssue(t, tc.issue, out.Issues)
		})
	}
}

func TestValidOwnerRepositoryAccess(t *testing.T) {
	tests := map[string]struct {
		owner string
		issue string
	}{
		"user with push":                    {owner: "@alice"},
		"user with maintain":                {owner: "@carol"},
		"user with admin":                   {owner: "@dave"},
		"user login in another letter case": {owner: "@ALICE"},
		"user without push": {
			owner: "@bob",
			issue: `User "bob" cannot review PRs on "repo" as they don't have write permissions.`,
		},
		"unknown user": {
			owner: "@eve",
			issue: `User "@eve" is not a member of the organization "org" or has no access to repository "repo"`,
		},
		"team with push":                      {owner: "@org/writers"},
		"team with maintain":                  {owner: "@org/maintainers"},
		"team and org in another letter case": {owner: "@ORG/Writers"},
		"team with only the push permission":  {owner: "@org/push-string"},
		"team with only the pull permission": {
			owner: "@org/pull-string",
			issue: `Team "pull-string" cannot review PRs on "repo" as neither it nor any parent team has write permissions.`,
		},
		"child team with inherited push access": {owner: "@org/child"},
		"team without push": {
			owner: "@org/readers",
			issue: `Team "readers" cannot review PRs on "repo" as neither it nor any parent team has write permissions.`,
		},
		"child team without push": {
			owner: "@org/child-reader",
			issue: `Team "child-reader" cannot review PRs on "repo" as neither it nor any parent team has write permissions.`,
		},
		"unknown team": {
			owner: "@org/nope",
			issue: `Team "@org/nope" does not exist in organization "org" or has no access to repository "repo"`,
		},
		"team of another organization": {
			owner: "@other/writers",
			issue: `Team "@other/writers" does not belong to "org" organization.`,
		},
		"email": {owner: "owner@example.com"},
	}

	gh, _ := newFakeGitHub(t, defaultRoutes())
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := newTestValidOwner(t, gh).Check(t.Context(), loadInput(t, "*\t"+tc.owner))
			assert.NilError(t, err)

			var want *Issue
			if tc.issue != "" {
				want = &Issue{Severity: Error, LineNo: 1, Message: tc.issue}
			}
			assertSingleIssue(t, want, out.Issues)
		})
	}
}

func TestValidOwnerChecksEachOwnerOnce(t *testing.T) {
	gh, fake := newFakeGitHub(t, defaultRoutes())
	v := newTestValidOwner(t, gh)
	in := loadInput(t, "*.go @eve @org/nope @alice @org/writers\n*.js @eve @org/nope @bob @org/readers")

	for range 2 {
		out, err := v.Check(t.Context(), in)
		assert.NilError(t, err)
		assert.DeepEqual(t, out.Issues, []Issue{
			{Severity: Error, LineNo: 1, Message: `User "@eve" is not a member of the organization "org" or has no access to repository "repo"`},
			{Severity: Error, LineNo: 1, Message: `Team "@org/nope" does not exist in organization "org" or has no access to repository "repo"`},
			{Severity: Error, LineNo: 2, Message: `User "bob" cannot review PRs on "repo" as they don't have write permissions.`},
			{
				Severity: Error, LineNo: 2,
				Message: `Team "readers" cannot review PRs on "repo" as neither it nor any parent team has write permissions.`,
			},
		})
	}

	assert.Equal(t, fake.count(routeCollaborators), 1, "collaborators are fetched once")
	assert.Equal(t, fake.count(routeTeams), 1, "teams are fetched once")
	assert.Equal(t, fake.count(routeTeamRepo), 2, "an unknown team is checked once per Check call")
}

func TestValidOwnerComparesOwnersCaseInsensitively(t *testing.T) {
	gh, fake := newFakeGitHub(t, defaultRoutes())
	v, err := NewValidOwner(ValidOwnerConfig{Repository: "org/repo", IgnoredOwners: []string{"@ghost"}}, gh)
	assert.NilError(t, err)

	out, err := v.Check(t.Context(), loadInput(t, "*.go @Ghost @eve\n*.js @EVE @GHOST"))
	assert.NilError(t, err)

	assert.DeepEqual(t, out.Issues, []Issue{
		{Severity: Error, LineNo: 1, Message: `User "@eve" is not a member of the organization "org" or has no access to repository "repo"`},
	})
	assert.Equal(t, fake.count(routeCollaborators), 1)
}

func TestValidOwnerListsAllPages(t *testing.T) {
	routes := defaultRoutes()
	routes[routeCollaborators] = func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, cmp.Equal(r.URL.Query().Get("affiliation"), "all"))
		assert.Check(t, cmp.Equal(r.URL.Query().Get("per_page"), "100"))

		if r.URL.Query().Get("page") == "" {
			next := *r.URL
			q := next.Query()
			q.Set("page", "2")
			next.RawQuery = q.Encode()
			w.Header().Set("Link", `<http://`+r.Host+next.String()+`>; rel="next"`)
			reply(http.StatusOK, "", collaboratorsJSON)(w, r)
			return
		}
		reply(http.StatusOK, "", `[{"login": "zed", "permissions": {"push": true}}]`)(w, r)
	}
	routes[routeTeams] = func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, cmp.Equal(r.URL.Query().Get("per_page"), "100"))
		reply(http.StatusOK, "", teamsJSON)(w, r)
	}
	gh, fake := newFakeGitHub(t, routes)

	out, err := newTestValidOwner(t, gh).Check(t.Context(), loadInput(t, "* @alice @zed @org/writers"))
	assert.NilError(t, err)
	assertSingleIssue(t, nil, out.Issues)
	assert.Equal(t, fake.count(routeCollaborators), 2)
}

func TestValidOwnerCheckSatisfied(t *testing.T) {
	t.Run("fine-grained token without scopes header", func(t *testing.T) {
		gh, _ := newFakeGitHub(t, defaultRoutes())
		assert.NilError(t, newTestValidOwner(t, gh).CheckSatisfied(t.Context()))
	})

	t.Run("classic token without read:org and team owners only", func(t *testing.T) {
		routes := defaultRoutes()
		routes[routeRepository] = reply(http.StatusOK, "repo", `{"name": "repo"}`)
		routes[routeCollaborators] = reply(http.StatusForbidden, "repo", `{"message": "Forbidden"}`)
		gh, fake := newFakeGitHub(t, routes)
		v := newTestValidOwner(t, gh)

		assert.NilError(t, v.CheckSatisfied(t.Context()))

		out, err := v.Check(t.Context(), loadInput(t, "* @org/writers @org/child"))
		assert.NilError(t, err)
		assertSingleIssue(t, nil, out.Issues)
		assert.Equal(t, fake.count(routeCollaborators), 0)
	})

	t.Run("repository not found", func(t *testing.T) {
		routes := defaultRoutes()
		routes[routeRepository] = reply(http.StatusNotFound, "read:org", notFoundJSON)
		gh, _ := newFakeGitHub(t, routes)

		err := newTestValidOwner(t, gh).CheckSatisfied(t.Context())
		assert.ErrorContains(t, err, "repository org/repo not found, or it's private and token doesn't have enough permission")
		assert.ErrorContains(t, err, `this token is missing "repo"`)
		assert.ErrorContains(t, err, `repository permission "Metadata" (read)`)
	})

	t.Run("unauthorized", func(t *testing.T) {
		routes := defaultRoutes()
		routes[routeRepository] = reply(http.StatusUnauthorized, "", `{"message": "Bad credentials"}`)
		gh, _ := newFakeGitHub(t, routes)

		err := newTestValidOwner(t, gh).CheckSatisfied(t.Context())
		assert.ErrorContains(t, err, "get the repository org/repo: GitHub authorization is required")
		assert.ErrorContains(t, err, "Bad credentials")
	})
}

func TestValidOwnerAPIErrors(t *testing.T) {
	rateLimited := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Ratelimit-Remaining", "0")
		reply(http.StatusForbidden, "", `{"message": "API rate limit exceeded"}`)(w, r)
	}

	tests := map[string]struct {
		route   string
		handler http.HandlerFunc
		want    []string
		notWant string
	}{
		"collaborators forbidden for a classic token without read:org": {
			route:   routeCollaborators,
			handler: reply(http.StatusForbidden, "repo", `{"message": "Must have push access to view repository collaborators."}`),
			want: []string{
				"list the collaborators of org/repo: ",
				"Must have push access to view repository collaborators.",
				`this token is missing "read:org"`,
				`repository permission "Metadata" (read)`,
			},
		},
		"collaborators unauthorized": {
			route:   routeCollaborators,
			handler: reply(http.StatusUnauthorized, "", `{"message": "Bad credentials"}`),
			want:    []string{"list the collaborators of org/repo: GitHub authorization is required", "Bad credentials", `"Metadata"`},
		},
		"collaborators rate limit": {
			route:   routeCollaborators,
			handler: rateLimited,
			want:    []string{"list the collaborators of org/repo: GitHub rate limit reached"},
			notWant: "Access needed",
		},
		"collaborators secondary rate limit": {
			route: routeCollaborators,
			handler: reply(http.StatusForbidden, "", `{"message": "You have exceeded a secondary rate limit.",
				"documentation_url": "https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits"}`),
			want:    []string{"list the collaborators of org/repo: GitHub rate limit reached"},
			notWant: "Access needed",
		},
		"collaborators server error": {
			route:   routeCollaborators,
			handler: reply(http.StatusInternalServerError, "", `{"message": "Server Error"}`),
			want:    []string{"list the collaborators of org/repo: ", "500"},
			notWant: "Access needed",
		},
		"teams forbidden": {
			route:   routeTeams,
			handler: reply(http.StatusForbidden, "", `{"message": "Resource not accessible by personal access token"}`),
			want:    []string{"list the teams of org/repo: ", `repository permission "Administration" (read)`},
		},
		"team repository forbidden": {
			route:   routeTeamRepo,
			handler: reply(http.StatusForbidden, "", `{"message": "Resource not accessible by integration"}`),
			want:    []string{`check the access of team "child" to org/repo: `, `organization permission "Members" (read)`},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			routes := defaultRoutes()
			routes[routeRepository] = reply(http.StatusOK, "repo", `{"name": "repo"}`)
			routes[tc.route] = tc.handler
			gh, _ := newFakeGitHub(t, routes)
			v := newTestValidOwner(t, gh)
			assert.NilError(t, v.CheckSatisfied(t.Context()))

			out, err := v.Check(t.Context(), loadInput(t, "*.md @other/team\n*.go @alice @org/child"))

			assert.Assert(t, err != nil)
			for _, want := range tc.want {
				assert.ErrorContains(t, err, want)
			}
			if tc.notWant != "" {
				assert.Assert(t, !strings.Contains(err.Error(), tc.notWant), "got %v", err)
			}

			// The issues found before the failure are returned.
			assertSingleIssue(t, &Issue{
				Severity: Error,
				LineNo:   1,
				Message:  `Team "@other/team" does not belong to "org" organization.`,
			}, out.Issues)
		})
	}
}

func TestValidOwnerCanceledContext(t *testing.T) {
	gh, fake := newFakeGitHub(t, defaultRoutes())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	out, err := newTestValidOwner(t, gh).Check(ctx, loadInput(t, "* @alice @org/writers"))

	assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
	assert.Equal(t, len(out.Issues), 0)
	assert.Equal(t, fake.count(routeCollaborators)+fake.count(routeTeams), 0)
}
