// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The GitHub client that implements the forge interface, as
// a port of @semantic-release/github.

// Package github is the GitHub implementation of the forge interface.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	giturls "github.com/chainguard-dev/git-urls"
	gh "github.com/google/go-github/v84/github"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
)

// These are the errors of the client.
var (
	// errRepoURL is returned for a repository URL without an owner and a
	// name.
	errRepoURL = errors.New("invalid repository URL")

	// errNoVisibility is returned when the repository response has
	// neither the visibility nor the private field.
	errNoVisibility = errors.New("the response has no visibility")
)

// releaseName names a GitHub release in success comments, as RELEASE_NAME
// in definitions/constants.js does.
const releaseName = "GitHub release"

// defaultBaseURL is the GitHub API URL that Octokit uses by default.
const defaultBaseURL = "https://api.github.com/"

// Options configures a Client.
type Options struct {
	// Token is the GitHub API token. The plugin reads GH_TOKEN, then
	// GITHUB_TOKEN (resolve-config.js).
	Token string

	// GitCredentials are the credentials that GitURL can put into the
	// repository URL, in the order to try them, such as
	// GitCredentialsFromEnv returns them. Empty means Token, when Token
	// is not empty.
	GitCredentials []string

	// VerifyAuth checks whether git can push to a URL, as git.js
	// verifyAuth does with "git push --dry-run". GitURL uses it to keep
	// a URL that works without credentials, and to choose between more
	// than one credential. Nil means that no URL is checked: GitURL then
	// uses the first credential.
	VerifyAuth func(ctx context.Context, url string) error

	// RepoURL is the configured repository URL (the git remote). The
	// owner and the name of the repository come from it.
	RepoURL string

	// BaseURL is the API base URL. Empty means https://api.github.com/.
	BaseURL string

	// Log, if not nil, receives the messages that the plugin logs.
	Log func(format string, args ...any)
}

// Client is the GitHub forge.
type Client struct {
	// opts are the options that New got.
	opts Options

	// gh is the go-github client.
	gh *gh.Client

	// owner and repo name the repository.
	owner, repo string

	// err is the error of New, which every API call returns. GitURL
	// works without the API, so New returns no error.
	err error

	// mu guards uploadURLs.
	mu sync.Mutex

	// uploadURLs maps a release ID to the upload_url template of the
	// release, as the API returned it.
	uploadURLs map[string]string
}

// _ checks that Client implements forge.Forge.
var _ forge.Forge = (*Client)(nil)

// New returns a Client. An invalid BaseURL, or a RepoURL without an
// owner and a name, makes every API call fail.
//
//nolint:gocritic // Why: Options is a value, as in the other clients' New.
func New(opts Options) *Client {
	c := &Client{opts: opts, uploadURLs: map[string]string{}}
	c.gh = gh.NewClient(&http.Client{Transport: &tokenTransport{token: opts.Token}})
	if c.owner, c.repo, c.err = parseRepo(opts.RepoURL); c.err != nil {
		return c
	}
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}
	base, err := url.Parse(opts.BaseURL)
	if err != nil {
		c.err = fmt.Errorf("parse GitHub API URL: %w", err)
		return c
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	c.gh.BaseURL = base
	return c
}

// ReleaseName returns the name of a GitHub release in success comments.
func (c *Client) ReleaseName() string {
	return releaseName
}

// Visibility returns the visibility of the repository that GitHub
// reports: "public", "private" or "internal". The visibility field is
// optional in the API description, so without it the required private
// field decides. A response with neither is an error.
func (c *Client) Visibility(ctx context.Context) (string, error) {
	r, err := c.getRepository(ctx)
	switch {
	case err != nil:
		return "", err
	case r.GetVisibility() != "":
		return r.GetVisibility(), nil
	case r.Private == nil:
		return "", apiError("get the visibility of "+c.owner+"/"+c.repo, errNoVisibility)
	case *r.Private:
		return "private", nil
	}
	return forge.Public, nil
}

// parseRepo returns the owner and the name of the repository at repoURL,
// as parse-github-url.js does: the first path element is the owner, and
// the rest, without ".git", is the name.
func parseRepo(repoURL string) (owner, repo string, err error) {
	u, err := giturls.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return "", "", fmt.Errorf("%w %q: %w", errRepoURL, repoURL, err)
	}
	owner, repo, _ = strings.Cut(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git"), "/")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("%w %q: no owner and name", errRepoURL, repoURL)
	}
	return owner, repo, nil
}

// logf logs a message, as the plugin's logger.log and logger.error do.
func (c *Client) logf(format string, args ...any) {
	if c.opts.Log != nil {
		c.opts.Log(format, args...)
	}
}

// apiError wraps an API error with forge.ErrForge and what failed.
func apiError(what string, err error) error {
	return fmt.Errorf("%s: %w: %w", what, forge.ErrForge, err)
}

// statusCode returns the HTTP status of an API error, or 0.
func statusCode(err error) int {
	var errResp *gh.ErrorResponse
	if errors.As(err, &errResp) && errResp.Response != nil {
		return errResp.Response.StatusCode
	}
	var rateErr *gh.RateLimitError
	if errors.As(err, &rateErr) && rateErr.Response != nil {
		return rateErr.Response.StatusCode
	}
	var abuseErr *gh.AbuseRateLimitError
	if errors.As(err, &abuseErr) && abuseErr.Response != nil {
		return abuseErr.Response.StatusCode
	}
	return 0
}

// repository returns the owner and name of the repository as GitHub
// reports them, because the repository can have a new name: "In case the
// repo changed name, get the new `repo`/`owner` as the search API will
// not follow redirects" (success.js, fail.js).
func (c *Client) repository(ctx context.Context) (owner, repo string, err error) {
	r, err := c.getRepository(ctx)
	if err != nil {
		return "", "", err
	}
	owner, repo, _ = strings.Cut(r.GetFullName(), "/")
	return owner, repo, nil
}

// getRepository gets the repository from the API.
func (c *Client) getRepository(ctx context.Context) (*gh.Repository, error) {
	if c.err != nil {
		return nil, c.err
	}
	r, _, err := c.gh.Repositories.Get(ctx, c.owner, c.repo)
	if err != nil {
		return nil, apiError("get repository "+c.owner+"/"+c.repo, err)
	}
	return r, nil
}

// graphqlResponse is the body of a GraphQL response.
type graphqlResponse struct {
	// Data is the result.
	Data json.RawMessage `json:"data"`

	// Errors are the errors, if any. A JSON null decodes as none.
	Errors []graphqlError `json:"errors"`
}

// graphqlError is one error of a GraphQL response.
type graphqlError struct {
	// Message describes the error.
	Message string `json:"message"`
}

// jsonNull is the JSON null, which is also how a JavaScript template
// string prints null.
const jsonNull = "null"

// errGraphQL is returned for a GraphQL response with errors.
var errGraphQL = errors.New("graphql response has errors")

// graphql sends a GraphQL query with variables, as octokit.graphql does,
// and decodes the data of the response into out. A response with errors
// fails, as in @octokit/graphql.
func (c *Client) graphql(ctx context.Context, query string, variables map[string]any, out any) error {
	if c.err != nil {
		return c.err
	}
	req, err := c.gh.NewRequest(http.MethodPost, "graphql", map[string]any{"query": query, "variables": variables})
	if err != nil {
		return fmt.Errorf("build GraphQL request: %w", err)
	}
	var resp graphqlResponse
	if _, err := c.gh.Do(ctx, req, &resp); err != nil {
		return apiError("GraphQL query", err)
	}
	if errs := resp.Errors; len(errs) > 0 {
		// This is the message of GraphqlResponseError.
		var msg strings.Builder
		msg.WriteString("Request failed due to following response errors:")
		for _, e := range errs {
			msg.WriteString("\n - " + e.Message)
		}
		return apiError("GraphQL query", fmt.Errorf("%w: %s", errGraphQL, msg.String()))
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("decode GraphQL data: %w", err)
	}
	return nil
}

// tokenTransport sets the Authorization header of each request, as
// @octokit/auth-token does.
type tokenTransport struct {
	// token is the API token, or empty for none.
	token string
}

// tokenPrefixRegexp matches a scheme in front of a token, which
// createTokenAuth removes. The JavaScript "i" flag folds only ASCII
// letters here, so the classes are spelled out: Go's (?i) would also
// match the Kelvin sign as "k".
var tokenPrefixRegexp = regexp.MustCompile(`^(?:[tT][oO][kK][eE][nN]|[bB][eE][aA][rR][eE][rR]) +`)

// RoundTrip sends req with the Authorization header: "bearer" for a
// token with three dot-separated parts (a JWT), else "token".
func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rlog.Debugf(req.Context(), "%s %s", req.Method, req.URL)
	base := http.DefaultTransport
	if t.token == "" {
		return base.RoundTrip(req)
	}
	token := tokenPrefixRegexp.ReplaceAllString(t.token, "")
	scheme := "token "
	if strings.Count(token, ".") == 2 {
		scheme = "bearer "
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", scheme+token)
	return base.RoundTrip(req)
}
