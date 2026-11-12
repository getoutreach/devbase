// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The git URL with GitHub credentials, as a port of the
// GitHub token parts of semantic-release's lib/get-git-auth-url.js.

package github

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/internal/js"
)

// GitCredentialsFromEnv returns the git credentials of the GitHub tokens
// in the environment, in the order of GIT_TOKENS in get-git-auth-url.js:
// GH_TOKEN, then GITHUB_TOKEN. In GitHub Actions (GITHUB_ACTION is set),
// GITHUB_TOKEN gets the "x-access-token:" user name. As upstream, a
// variable that is set to an empty value counts.
func GitCredentialsFromEnv(lookup func(name string) (string, bool)) []string {
	var creds []string
	if token, ok := lookup("GH_TOKEN"); ok {
		creds = append(creds, token)
	}
	if token, ok := lookup("GITHUB_TOKEN"); ok {
		if _, ok := lookup("GITHUB_ACTION"); ok {
			token = "x-access-token:" + token
		}
		creds = append(creds, token)
	}
	return creds
}

// GitURL returns the URL that git uses to fetch and push, as
// get-git-auth-url.js does: it expands a GitHub shortcut ("owner/repo")
// and turns "git+https" into "https"; it keeps that URL when
// Options.VerifyAuth accepts it; else it puts the credentials into an
// https (or http) URL, with VerifyAuth to choose between more than one.
// Without credentials, or when VerifyAuth accepts none, it returns the
// URL without credentials.
func (c *Client) GitURL(ctx context.Context) (string, error) {
	repoURL := c.opts.RepoURL
	protocol := urlProtocol(repoURL)
	if short, ok := githubShortcutURL(repoURL); ok {
		repoURL = short
	} else if strings.Contains(protocol, "http") {
		scheme := "http"
		if strings.Contains(protocol, "https") {
			scheme = "https"
		}
		u, err := parseNodeURL(repoURL)
		if err != nil {
			return "", err
		}
		repoURL = u.format(scheme, u.auth, u.host())
	}

	verify := c.opts.VerifyAuth
	if verify != nil && verify(ctx, repoURL) == nil {
		return repoURL, nil
	}

	creds := c.opts.GitCredentials
	if len(creds) == 0 && c.opts.Token != "" {
		creds = []string{c.opts.Token}
	}
	// "Skip verification if there is no ambiguity on which env var to
	// use for authentication."
	if len(creds) == 1 {
		return formatAuthURL(protocol, repoURL, creds[0])
	}
	for _, cred := range creds {
		authURL, err := formatAuthURL(protocol, repoURL, cred)
		if err != nil {
			return "", err
		}
		if verify == nil || verify(ctx, authURL) == nil {
			return authURL, nil
		}
	}
	return repoURL, nil
}

// protocolRegexp matches the protocol of Node's legacy url.parse.
var protocolRegexp = regexp.MustCompile(`^[a-zA-Z0-9.+-]+:`)

// urlProtocol returns the protocol that Node's url.parse finds in s,
// such as "https:" or "git+https:", in lowercase, or "" for none. An scp
// style URL with a user ("git@host:path") has none.
func urlProtocol(s string) string {
	return strings.ToLower(protocolRegexp.FindString(strings.TrimSpace(s)))
}

// githubShortcutURL returns the https URL of a GitHub shortcut, such as
// "owner/repo" or "github:owner/repo#ref", as hosted-git-info's
// https({noGitPlus: true}) does. The GitLab, Bitbucket and gist
// shortcuts are not GitHub's and are not supported.
func githubShortcutURL(s string) (string, bool) {
	if isGitHubShorthand(s) {
		s = "github:" + s
	}
	if len(s) < len("github:") || !strings.EqualFold(s[:len("github:")], "github:") {
		return "", false
	}
	pathname, committish, _ := strings.Cut(s[len("github:"):], "#")
	pathname = strings.TrimPrefix(pathname, "/")
	if _, after, found := strings.Cut(pathname, "@"); found {
		pathname = after
	}
	// A JavaScript template string prints a missing user as "null".
	user, project := jsonNull, pathname
	if i := strings.LastIndex(pathname, "/"); i >= 0 {
		user, project = pathname[:i], pathname[i+1:]
		if user == "" {
			user = jsonNull
		}
	}
	project = strings.TrimSuffix(project, ".git")
	u := "https://github.com/" + user + "/" + project + ".git"
	if committish != "" {
		u += "#" + committish
	}
	return u, true
}

// isGitHubShorthand reports whether s is a GitHub shorthand such as
// "owner/repo", as from-url.js in hosted-git-info decides it.
func isGitHubShorthand(s string) bool {
	firstHash := strings.Index(s, "#")
	firstSlash := strings.Index(s, "/")
	secondSlash := -1
	if i := strings.Index(s[firstSlash+1:], "/"); i >= 0 {
		secondSlash = firstSlash + 1 + i
	}
	firstSpace := strings.IndexFunc(s, js.IsSpace)
	afterHash := func(i int) bool { return i == -1 || (firstHash > -1 && i > firstHash) }
	doesNotEndWithSlash := !strings.HasSuffix(s, "/")
	if firstHash > -1 {
		doesNotEndWithSlash = firstHash == 0 || s[firstHash-1] != '/'
	}
	return afterHash(firstSpace) && firstSlash > 0 && doesNotEndWithSlash && !strings.HasPrefix(s, ".") &&
		afterHash(strings.Index(s, "@")) && afterHash(strings.Index(s, ":")) && afterHash(secondSlash)
}

// scpRegexp matches an scp style URL, such as "git@host:owner/repo.git"
// or "host:6666:/owner/repo.git", as in formatAuthUrl.
var scpRegexp = regexp.MustCompile(`^(?:(.*)@)?(.*?):(\d+)?:?/?(.*)$`)

// httpProtocolRegexp matches a protocol that formatAuthUrl keeps as
// http.
var httpProtocolRegexp = regexp.MustCompile(`http[^s]`)

// formatAuthURL puts credentials into a repository URL, as formatAuthUrl
// does: an scp style URL becomes an ssh URL first; the result is https,
// or http for an http protocol; an ssh URL loses its port.
func formatAuthURL(protocol, repoURL, credentials string) (string, error) {
	s := repoURL
	// The upstream regular expression starts with (?!.+:\/\/): no "://"
	// after the first character.
	if repoURL == "" || !strings.Contains(repoURL[1:], "://") {
		if m := scpRegexp.FindStringSubmatch(repoURL); m != nil {
			s = "ssh://"
			if m[1] != "" {
				s += m[1] + "@"
			}
			s += m[2]
			if m[3] != "" {
				s += ":" + m[3]
			}
			s += "/" + m[4]
		}
	}
	u, err := parseNodeURL(s)
	if err != nil {
		return "", err
	}
	host := u.hostname
	if protocol != "ssh:" && u.port != "" {
		host += ":" + u.port
	}
	scheme := "https"
	if httpProtocolRegexp.MatchString(protocol) {
		scheme = "http"
	}
	return u.format(scheme, credentials, host), nil
}

// nodeURL is a URL as Node's legacy url.parse splits it.
type nodeURL struct {
	// auth is the decoded "user:password" part.
	auth string

	// hostname is the host in lowercase, or "null" for none, as a
	// template string prints Node's null.
	hostname string

	// port is the port, or empty.
	port string

	// rest is the path, query and fragment.
	rest string
}

// parseNodeURL splits s as Node's url.parse does.
func parseNodeURL(s string) (*nodeURL, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("parse repository URL: %w", err)
	}
	n := &nodeURL{hostname: strings.ToLower(u.Hostname()), port: u.Port(), rest: u.EscapedPath()}
	if n.hostname == "" {
		n.hostname = jsonNull
	}
	if u.User != nil {
		n.auth = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			n.auth += ":" + pw
		}
	}
	if u.ForceQuery || u.RawQuery != "" {
		n.rest += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		n.rest += "#" + u.EscapedFragment()
	}
	return n, nil
}

// host returns the host and port.
func (n *nodeURL) host() string {
	if n.port == "" {
		return n.hostname
	}
	return n.hostname + ":" + n.port
}

// format returns the URL with scheme, auth and host, as Node's
// url.format does: it percent-encodes auth, except the characters that
// encodeURIComponent keeps and ":".
func (n *nodeURL) format(scheme, auth, host string) string {
	s := scheme + "://"
	if auth != "" {
		s += js.PercentEncode(auth, js.URIAuthKeep) + "@"
	}
	return s + host + n.rest
}

// encodeUnreserved percent-encodes s as Octokit's URL templates do: only
// the RFC 3986 unreserved characters stay.
func encodeUnreserved(s string) string {
	return js.PercentEncode(s, "-_.~")
}
