// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the secret masking and the debug log. The
// masking tests port test/hide-sensitive.test.js of semantic-release
// 25.0.9.

package rlog

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/internal/js"
	"gotest.tools/v3/assert"
)

// gitAuthURL is the URL with credentials that get-git-auth-url.js makes
// with url.format, as the upstream test builds it.
func gitAuthURL(auth string) string {
	return "https://" + js.PercentEncode(auth, js.URIAuthKeep) + "@host.com/owner/repo.git"
}

func TestMask(t *testing.T) {
	const s = Secure
	type tc struct {
		name     string
		env      []string
		in, want string
	}
	tests := []tc{
		{
			"Replace multiple sensitive environment variable values",
			[]string{"SOME_PASSWORD=password", "SOME_TOKEN=secret"},
			"https://user:password@host.com?token=secret", "https://user:" + s + "@host.com?token=" + s,
		},
		{
			"Replace multiple occurrences of sensitive environment variable values",
			[]string{"secretKey=secret"},
			"https://user:secret@host.com?token=secret", "https://user:" + s + "@host.com?token=" + s,
		},
		{
			`Replace sensitive environment variable matching specific regex for "private"`,
			[]string{"privateKey=secret", "GOPRIVATE=host.com"},
			"https://host.com?token=secret", "https://host.com?token=" + s,
		},
		{
			"Replace url-encoded environment variable",
			[]string{"privateKey=secret "},
			"https://host.com?token=" + js.EncodeURI("secret "), "https://host.com?token=" + s,
		},
		{
			"Escape regexp special characters",
			[]string{`SOME_CREDENTIALS=p$^{.+}\w[a-z]o.*rd`},
			`https://user:p$^{.+}\w[a-z]o.*rd@host.com`, "https://user:" + s + "@host.com",
		},
		{
			"Escape regexp special characters in url-encoded environment variable",
			[]string{`SOME_PASSWORD=secret password p$^{.+}\w[a-z]o.*rd)(`},
			"https://user:" + js.EncodeURI(`secret password p$^{.+}\w[a-z]o.*rd)(`) + "@host.com",
			"https://user:" + s + "@host.com",
		},
		{"Return same string if no environment variable has to be replaced", nil, "test", "test"},
		{
			"Exclude empty environment variables from the regexp",
			[]string{"SOME_PASSWORD=password", "SOME_TOKEN="},
			"https://user:password@host.com?token=", "https://user:" + s + "@host.com?token=",
		},
		{
			"Exclude empty environment variables from the regexp if there is only empty ones",
			[]string{"SOME_PASSWORD=", "SOME_TOKEN= \n "},
			"https://host.com?token=", "https://host.com?token=",
		},
		{
			"Exclude nonsensitive GOPRIVATE environment variable for Golang projects from the regexp",
			[]string{"GOPRIVATE=host.com"},
			"https://host.com?token=", "https://host.com?token=",
		},
		{
			"Exclude environment variables with value shorter than SECRET_MIN_SIZE from the regexp",
			[]string{"SHORT_TOKEN=aaaa", "LONG_TOKEN=bbbbb"},
			"https://user:aaaa@host.com?token=bbbbb", "https://user:aaaa@host.com?token=" + s,
		},
		{
			"Mask GIT_CREDENTIALS containing multiple `:` in a git authentication URL",
			[]string{"GIT_CREDENTIALS=user:pa:ss@word-secret"},
			gitAuthURL("user:pa:ss@word-secret"), "https://" + s + "@host.com/owner/repo.git",
		},
		{
			"Mask GITHUB_TOKEN containing reserved characters in a git authentication URL",
			[]string{"GITHUB_TOKEN=ghp_abc@def/secret?x=1&y=2"},
			gitAuthURL("x-access-token:ghp_abc@def/secret?x=1&y=2"), "https://x-access-token:" + s + "@host.com/owner/repo.git",
		},
		{
			"Mask GH_TOKEN containing reserved characters in a git authentication URL",
			[]string{"GH_TOKEN=ghp_abc@def-secret"},
			gitAuthURL("ghp_abc@def-secret"), "https://" + s + "@host.com/owner/repo.git",
		},
		{
			"Mask GL_TOKEN containing `:` and reserved characters in a git authentication URL",
			[]string{"GL_TOKEN=glpat-abc:def@x-secret"},
			gitAuthURL("gitlab-ci-token:glpat-abc:def@x-secret"), "https://gitlab-ci-token:" + s + "@host.com/owner/repo.git",
		},
		{
			"Mask fully percent-encoded secrets",
			[]string{"SOME_PASSWORD=user:pass@word-secret"},
			"token=" + js.EncodeURIComponent("user:pass@word-secret"), "token=" + s,
		},
		{
			"Mask multiple secrets with reserved characters",
			[]string{"SOME_PASSWORD=pass@word", "SOME_TOKEN=token/value"},
			"https://user:" + js.EncodeURIComponent("pass@word") + "@host.com?token=" + js.EncodeURIComponent("token/value"),
			"https://user:" + s + "@host.com?token=" + s,
		},
		{
			"Mask encoded credentials in git command output",
			[]string{"GIT_CREDENTIALS=user:abc@def-secret"},
			"git ls-remote --heads '" + gitAuthURL("user:abc@def-secret") + "'",
			"git ls-remote --heads 'https://" + s + "@host.com/owner/repo.git'",
		},
		{
			"Mask multiple occurrences of encoded credentials",
			[]string{"secretKey=abc@def-secret"},
			"url1: " + gitAuthURL("abc@def-secret") + " url2: " + gitAuthURL("abc@def-secret"),
			"url1: https://" + s + "@host.com/owner/repo.git url2: https://" + s + "@host.com/owner/repo.git",
		},
		{
			"Mask fully percent-encoded API_KEY values",
			[]string{"API_KEY=user:pass@word-secret"},
			"key=" + js.EncodeURIComponent("user:pass@word-secret"), "key=" + s,
		},
		{
			// A later item replaces the value of an earlier one, as in an
			// object.
			"Last value wins",
			[]string{"SOME_TOKEN=first-value", "SOME_TOKEN=second-value"},
			"first-value second-value", "first-value " + s,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, NewMasker(tc.env).Mask(tc.in), tc.want)
		})
	}
	for _, c := range []string{"@", "/", "?", "&", "=", "#", " "} {
		value := "user:abc" + c + "def-secret"
		t.Run(fmt.Sprintf("Mask GIT_CREDENTIALS containing %q in a git authentication URL", c), func(t *testing.T) {
			assert.Equal(t, NewMasker([]string{"GIT_CREDENTIALS=" + value}).Mask(gitAuthURL(value)),
				"https://"+s+"@host.com/owner/repo.git")
		})
	}
	for _, name := range []string{
		"API_KEY", "DEPLOY_KEY", "SIGNING_KEY", "SSH_KEY", "ENCRYPTION_KEY", "AUTH", "BASIC_AUTH",
		"SLACK_WEBHOOK", "DOCKER_AUTH", "AWS_ACCESS_KEY_ID",
	} {
		value := "SUPER-SECRET-VALUE-" + name
		t.Run("Mask "+name+" values", func(t *testing.T) {
			assert.Equal(t, NewMasker([]string{name + "=" + value}).Mask("leaked value: "+value), "leaked value: "+s)
		})
	}
}

func TestMaskerWriter(t *testing.T) {
	var b bytes.Buffer
	w := NewMasker([]string{"GH_TOKEN=ghp_secret"}).Writer(&b)
	n, err := fmt.Fprint(w, "push https://ghp_secret@github.com\n")
	assert.NilError(t, err)
	assert.Equal(t, n, len("push https://ghp_secret@github.com\n"))
	assert.Equal(t, b.String(), "push https://"+Secure+"@github.com\n")

	// Without secrets, there is no Masker, and the writer is w itself.
	assert.Assert(t, NewMasker([]string{"PATH=/usr/bin"}) == nil)
	assert.Equal(t, NewMasker(nil).Writer(&b), &b)
	assert.Equal(t, NewMasker(nil).Mask("text"), "text")
}

func TestDebugf(t *testing.T) {
	// Without a debug log, Debugf does nothing.
	Debugf(context.Background(), "nothing %d", 1)

	assert.Assert(t, !DebugEnabled(context.Background()))
	var b bytes.Buffer
	ctx := WithDebug(context.Background(), &b)
	assert.Assert(t, DebugEnabled(ctx))
	Debugf(ctx, "git %s", "fetch --tags")
	assert.Equal(t, b.String(), "debug: git fetch --tags\n")

	// The Masker of the context masks the debug log.
	b.Reset()
	m := NewMasker([]string{"GH_TOKEN=ghp_secret"})
	assert.Equal(t, MaskerFrom(WithMasker(ctx, m)), m)
	Debugf(WithMasker(ctx, m), "git push https://ghp_secret@github.com")
	assert.Equal(t, b.String(), "debug: git push https://"+Secure+"@github.com\n")
}
