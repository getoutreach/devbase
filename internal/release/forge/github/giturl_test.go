// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of GitURL, ported from test/get-git-auth-url.test.js
// of semantic-release 25.0.9.

package github

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// errNoPush is the error of a VerifyAuth that rejects every URL, as git
// does in the repositories of the upstream tests, which have no remote.
var errNoPush = errors.New("push rejected")

// denyPush is a VerifyAuth that rejects every URL.
func denyPush(context.Context, string) error {
	return errNoPush
}

// TestGitURL ports the cases of get-git-auth-url.test.js that apply to
// GitHub. The GIT_CREDENTIALS cases test the URL formatting with the
// "user:pass" credentials; GitCredentialsFromEnv does not read
// GIT_CREDENTIALS, which is not a GitHub variable.
func TestGitURL(t *testing.T) {
	tests := []struct {
		// name is the upstream test title.
		name string

		// repoURL is the repositoryUrl option.
		repoURL string

		// env is the environment of GitCredentialsFromEnv, or nil to use
		// creds.
		env map[string]string

		// creds are the git credentials when env is nil.
		creds []string

		// want is the expected URL.
		want string
	}{
		{
			name:    `Return the same "git" formatted URL if "gitCredentials" is not defined`,
			repoURL: "git@host.null:owner/repo.git",
			want:    "git@host.null:owner/repo.git",
		},
		{
			name:    `Return the same "https" formatted URL if "gitCredentials" is not defined`,
			repoURL: "https://host.null/owner/repo.git",
			want:    "https://host.null/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is not defined and repositoryUrl is a "git+https" URL`,
			repoURL: "git+https://host.null/owner/repo.git",
			want:    "https://host.null/owner/repo.git",
		},
		{
			name:    `Do not add trailing ".git" if not present in the origian URL`,
			repoURL: "git@host.null:owner/repo",
			want:    "git@host.null:owner/repo",
		},
		{
			name:    `Handle "https" URL with group and subgroup`,
			repoURL: "https://host.null/group/subgroup/owner/repo.git",
			want:    "https://host.null/group/subgroup/owner/repo.git",
		},
		{
			name:    `Handle "git" URL with group and subgroup`,
			repoURL: "git@host.null:group/subgroup/owner/repo.git",
			want:    "git@host.null:group/subgroup/owner/repo.git",
		},
		{
			name:    "Convert shorthand URL",
			repoURL: "semantic-release/semantic-release",
			want:    "https://github.com/semantic-release/semantic-release.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is defined and repositoryUrl is a "git" URL`,
			repoURL: "git@host.null:owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is defined and repositoryUrl is a "git" URL without user`,
			repoURL: "host.null:owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/owner/repo.git",
		},
		{
			name: `Return the "https" formatted URL if "gitCredentials" is defined and repositoryUrl is a "git" URL ` +
				`without user and with a custom port`,
			repoURL: "host.null:6666:owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null:6666/owner/repo.git",
		},
		{
			name: `Return the "https" formatted URL if "gitCredentials" is defined and repositoryUrl is a "git" URL ` +
				`without user and with a custom port followed by a slash`,
			repoURL: "host.null:6666:/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null:6666/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is defined and repositoryUrl is a "https" URL`,
			repoURL: "https://host.null/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/owner/repo.git",
		},
		{
			name:    `Return the "http" formatted URL if "gitCredentials" is defined and repositoryUrl is a "http" URL`,
			repoURL: "http://host.null/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "http://user:pass@host.null/owner/repo.git",
		},
		{
			name:    `Return the "http" formatted URL if "gitCredentials" is defined and repositoryUrl is a "http" URL with custom port`,
			repoURL: "http://host.null:8080/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "http://user:pass@host.null:8080/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is defined and repositoryUrl is a "git+https" URL`,
			repoURL: "git+https://host.null/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/owner/repo.git",
		},
		{
			name:    `Return the "http" formatted URL if "gitCredentials" is defined and repositoryUrl is a "git+http" URL`,
			repoURL: "git+http://host.null/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "http://user:pass@host.null/owner/repo.git",
		},
		{
			name:    `Return the "http" formatted URL if "gitCredentials" is defined and repositoryUrl is a "ssh" URL`,
			repoURL: "ssh://git@host.null:2222/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is defined with "GH_TOKEN"`,
			repoURL: "git@host.null:owner/repo.git",
			env:     map[string]string{"GH_TOKEN": "token"},
			want:    "https://token@host.null/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "gitCredentials" is defined with "GITHUB_TOKEN"`,
			repoURL: "git@host.null:owner/repo.git",
			env:     map[string]string{"GITHUB_TOKEN": "token"},
			want:    "https://token@host.null/owner/repo.git",
		},
		{
			name:    `Return the "https" formatted URL if "GITHUB_ACTION" is set`,
			repoURL: "git@host.null:owner/repo.git",
			env:     map[string]string{"GITHUB_ACTION": "foo", "GITHUB_TOKEN": "token"},
			want:    "https://x-access-token:token@host.null/owner/repo.git",
		},
		{
			name:    `Handle "https" URL with group and subgroup, with "GIT_CREDENTIALS"`,
			repoURL: "https://host.null/group/subgroup/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/group/subgroup/owner/repo.git",
		},
		{
			name:    `Handle "git" URL with group and subgroup, with "GIT_CREDENTIALS`,
			repoURL: "git@host.null:group/subgroup/owner/repo.git",
			creds:   []string{"user:pass"},
			want:    "https://user:pass@host.null/group/subgroup/owner/repo.git",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds := tc.creds
			if tc.env != nil {
				creds = GitCredentialsFromEnv(func(name string) (string, bool) {
					v, ok := tc.env[name]
					return v, ok
				})
			}
			c := New(Options{RepoURL: tc.repoURL, GitCredentials: creds, VerifyAuth: denyPush})
			got, err := c.GitURL(context.Background())
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

// TestGitURLPushAllowed ports "Do not add git credential to
// repositoryUrl if push is allowed" of get-git-auth-url.test.js.
func TestGitURLPushAllowed(t *testing.T) {
	var verified []string
	c := New(Options{
		RepoURL:        "git@host.null:owner/repo.git",
		GitCredentials: []string{"user:pass"},
		VerifyAuth: func(_ context.Context, u string) error {
			verified = append(verified, u)
			return nil
		},
	})
	got, err := c.GitURL(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, got, "git@host.null:owner/repo.git")
	assert.DeepEqual(t, verified, []string{"git@host.null:owner/repo.git"})
}

// TestGitURLManyCredentials checks the case of get-git-auth-url.js with
// both GH_TOKEN and GITHUB_TOKEN: it uses the first credential that git
// accepts, in the order of GIT_TOKENS, or the URL without credentials
// when git accepts none. Without VerifyAuth it uses the first one.
func TestGitURLManyCredentials(t *testing.T) {
	creds := GitCredentialsFromEnv(func(name string) (string, bool) {
		v, ok := map[string]string{"GH_TOKEN": "gh", "GITHUB_TOKEN": "github", "GITHUB_ACTION": ""}[name]
		return v, ok
	})
	assert.DeepEqual(t, creds, []string{"gh", "x-access-token:github"})

	var verified []string
	c := New(Options{
		RepoURL:        "https://github.com/owner/repo.git",
		GitCredentials: creds,
		VerifyAuth: func(_ context.Context, u string) error {
			verified = append(verified, u)
			if u == "https://x-access-token:github@github.com/owner/repo.git" {
				return nil
			}
			return errNoPush
		},
	})
	got, err := c.GitURL(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, got, "https://x-access-token:github@github.com/owner/repo.git")
	assert.DeepEqual(t, verified, []string{
		"https://github.com/owner/repo.git",
		"https://gh@github.com/owner/repo.git",
		"https://x-access-token:github@github.com/owner/repo.git",
	})

	c = New(Options{RepoURL: "https://github.com/owner/repo.git", GitCredentials: creds, VerifyAuth: denyPush})
	got, err = c.GitURL(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, got, "https://github.com/owner/repo.git")

	c = New(Options{RepoURL: "https://github.com/owner/repo.git", GitCredentials: creds})
	got, err = c.GitURL(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, got, "https://gh@github.com/owner/repo.git")
}

// TestGitURLToken checks that Options.Token is the credential when
// GitCredentials is empty.
func TestGitURLToken(t *testing.T) {
	c := New(Options{RepoURL: "git@github.com:owner/repo.git", Token: "ghp_abc"})
	got, err := c.GitURL(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, got, "https://ghp_abc@github.com/owner/repo.git")
}

// TestFormatAuthURL checks more URL forms against the output of
// formatAuthUrl and hosted-git-info in Node.js, with the credentials
// "x-access-token:tok".
func TestFormatAuthURL(t *testing.T) {
	for repoURL, want := range map[string]string{
		"owner/repo":                          "https://x-access-token:tok@github.com/owner/repo.git",
		"github:owner/repo":                   "https://x-access-token:tok@github.com/owner/repo.git",
		"owner/repo#main":                     "https://x-access-token:tok@github.com/owner/repo.git#main",
		"o/r.git":                             "https://x-access-token:tok@github.com/o/r.git",
		"git@github.com:o/r.git":              "https://x-access-token:tok@github.com/o/r.git",
		"https://github.com/o/r":              "https://x-access-token:tok@github.com/o/r",
		"https://user:pw@github.com/o/r.git":  "https://x-access-token:tok@github.com/o/r.git",
		"https://GitHub.com/O/R.git":          "https://x-access-token:tok@github.com/O/R.git",
		"git@github.com:123/r.git":            "https://x-access-token:tok@github.com:123/r.git",
		"git+ssh://git@github.com/o/r.git":    "https://x-access-token:tok@github.com/o/r.git",
		"ssh://github.com/o/r":                "https://x-access-token:tok@github.com/o/r",
		"https://github.com:443/o/r.git":      "https://x-access-token:tok@github.com:443/o/r.git",
		"git://github.com/o/r.git":            "https://x-access-token:tok@github.com/o/r.git",
		"https://github.com/o/r.git?x=1#frag": "https://x-access-token:tok@github.com/o/r.git?x=1#frag",
	} {
		c := New(Options{RepoURL: repoURL, GitCredentials: []string{"x-access-token:tok"}})
		got, err := c.GitURL(context.Background())
		assert.Check(t, err, repoURL)
		assert.Check(t, cmp.Equal(got, want), repoURL)
	}

	// url.format of Node.js 22 percent-encodes the credentials, except
	// the characters that encodeURIComponent keeps and ":".
	got, err := formatAuthURL("https:", "https://github.com/o/r.git", "us er:p@ss:x")
	assert.NilError(t, err)
	assert.Equal(t, got, "https://us%20er:p%40ss:x@github.com/o/r.git")
}
