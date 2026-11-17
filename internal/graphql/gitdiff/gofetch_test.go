// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the go-git fetch of a blob that is missing from a
// partial clone, against git http-backend.

package gitdiff

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"gotest.tools/v3/assert"
)

// baseContent is the content of schema.graphql at the merge-base of newTestRepo. A blobless clone
// that checks out "feature" does not have its blob.
const baseContent = "type Foo { a: String }\n"

// newHTTPRemote serves a bare copy of the repository at source over http with git http-backend,
// and returns the URL of the copy. When allowAnySHA1 is false, the server does not let a client
// fetch an object by its hash with protocol v1, which is the protocol of go-git. When wrap is not
// nil, it wraps the handler of the server.
func newHTTPRemote(t *testing.T, source string, allowAnySHA1 bool, wrap func(http.Handler) http.Handler) string {
	t.Helper()

	projectRoot := t.TempDir()
	bare := filepath.Join(projectRoot, "repo.git")
	runGit(t, projectRoot, "clone", "-q", "--bare", source, bare)
	runGit(t, bare, "config", "uploadpack.allowFilter", "true")
	if allowAnySHA1 {
		runGit(t, bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	}

	execPath, err := gitOutput(t, ".", "--exec-path")
	assert.NilError(t, err)
	var handler http.Handler = &cgi.Handler{
		Path: filepath.Join(execPath, "git-http-backend"),
		Env: []string{
			"GIT_PROJECT_ROOT=" + projectRoot,
			"GIT_HTTP_EXPORT_ALL=1",
			"GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_CONFIG_NOSYSTEM=1",
		},
	}
	if wrap != nil {
		handler = wrap(handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server.URL + "/repo.git"
}

// cloneBlobless makes a blobless clone of the repository at url and checks out branch.
func cloneBlobless(t *testing.T, url, branch string) string {
	t.Helper()

	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(clone), "clone", "-q", "--filter=blob:none", "--no-checkout", url, clone)
	runGit(t, clone, "checkout", "-q", branch)

	return clone
}

// baseBlobIsMissing reports whether the blob of baseContent is missing from the clone. rev-list
// prints a missing object as "?<hash>" and does not fetch it.
func baseBlobIsMissing(t *testing.T, clone string) bool {
	t.Helper()

	objects, err := gitOutput(t, clone, "rev-list", "--objects", "--missing=print", "HEAD")
	assert.NilError(t, err)

	return strings.Contains(objects, "?"+blobHash(baseContent))
}

// clearTokens removes the GitHub tokens of the environment for the test.
func clearTokens(t *testing.T) {
	t.Helper()

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("OUTREACH_GITHUB_TOKEN", "")
}

// TestMergeBaseFilesGoGitFetch checks that go-git fetches a missing blob from an http remote. The
// blob is still missing from the clone afterwards, so git did not fetch it.
func TestMergeBaseFilesGoGitFetch(t *testing.T) {
	clearTokens(t)
	root, _, baseName := newTestRepo(t)
	clone := cloneBlobless(t, newHTTPRemote(t, root, true, nil), "feature")
	assert.Assert(t, baseBlobIsMissing(t, clone), "the blob is already in the clone")

	content, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.NilError(t, err)
	assert.Equal(t, content["schema.graphql"], baseContent)
	assert.Assert(t, baseBlobIsMissing(t, clone), "git, not go-git, fetched the blob")
}

// TestMergeBaseFilesGoGitFallback checks that git fetches the blob when go-git cannot, because the
// server does not let go-git fetch an object by its hash.
func TestMergeBaseFilesGoGitFallback(t *testing.T) {
	clearTokens(t)
	root, _, baseName := newTestRepo(t)
	url := newHTTPRemote(t, root, false, nil)
	clone := cloneBlobless(t, url, "feature")
	assert.Assert(t, baseBlobIsMissing(t, clone), "the blob is already in the clone")

	_, err := blobViaGoGit(t.Context(), url, plumbing.NewHash(blobHash(baseContent)))
	assert.Assert(t, err != nil, "go-git fetched the blob, so the fallback is not tested")

	content, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.NilError(t, err)
	assert.Equal(t, content["schema.graphql"], baseContent)
	assert.Assert(t, !baseBlobIsMissing(t, clone), "git did not fetch the blob")
}

// TestFetchBlobSendsToken checks that the token candidate authenticates with a server that
// requires it, and that the fetch fails without it.
func TestFetchBlobSendsToken(t *testing.T) {
	const token = "test-token"
	root, _, _ := newTestRepo(t)

	var gotUser, gotPassword string
	url := newHTTPRemote(t, root, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, password, ok := r.BasicAuth()
			if ok {
				gotUser, gotPassword = user, password
			}
			if !ok || user != "x-access-token" || password != token {
				w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	hash := plumbing.NewHash(blobHash(baseContent))

	_, err := fetchBlob(t.Context(), url, hash, []authCandidate{{kind: "none"}})
	assert.Assert(t, err != nil, "the server did not require the token")

	tokenAuth := &githttp.BasicAuth{Username: "x-access-token", Password: token}
	content, err := fetchBlob(t.Context(), url, hash, []authCandidate{{kind: "token", auth: tokenAuth}})
	assert.NilError(t, err)
	assert.Equal(t, content, baseContent)
	assert.Equal(t, gotUser, "x-access-token")
	assert.Equal(t, gotPassword, token)
}

// candidateKinds returns the kinds of candidates.
func candidateKinds(candidates []authCandidate) []string {
	kinds := make([]string, 0, len(candidates))
	for _, c := range candidates {
		kinds = append(kinds, c.kind)
	}

	return kinds
}

// newEndpoint parses url as a git remote URL.
func newEndpoint(t *testing.T, url string) *transport.Endpoint {
	t.Helper()

	ep, err := transport.NewEndpoint(url)
	assert.NilError(t, err)

	return ep
}

func TestAuthCandidatesHTTP(t *testing.T) {
	const githubURL = "https://github.com/getoutreach/devbase.git"

	tests := []struct {
		name, url, githubToken, outreachToken string
		wantToken                             string
	}{
		{name: "GITHUB_TOKEN", url: githubURL, githubToken: "a", outreachToken: "b", wantToken: "a"},
		{name: "OUTREACH_GITHUB_TOKEN", url: githubURL, outreachToken: "b", wantToken: "b"},
		{name: "no token", url: githubURL},
		{name: "a host that is not GitHub", url: "https://example.com/repo.git", githubToken: "a"},
		{name: "plain http", url: "http://github.com/getoutreach/devbase.git", githubToken: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tt.githubToken)
			t.Setenv("OUTREACH_GITHUB_TOKEN", tt.outreachToken)

			candidates := authCandidates(newEndpoint(t, tt.url))
			assert.Equal(t, len(candidates), 1)
			if tt.wantToken == "" {
				assert.DeepEqual(t, candidateKinds(candidates), []string{"none"})
				assert.Assert(t, candidates[0].auth == nil)
				return
			}
			assert.DeepEqual(t, candidateKinds(candidates), []string{"token"})
			assert.DeepEqual(t, candidates[0].auth, &githttp.BasicAuth{Username: "x-access-token", Password: tt.wantToken})
		})
	}
}

// listenUnix listens on a new unix socket and returns its path. The path is short, because a unix
// socket path has a small length limit.
func listenUnix(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "agent")
	assert.NilError(t, err)
	t.Cleanup(func() { assert.NilError(t, os.RemoveAll(dir)) })

	path := filepath.Join(dir, "sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	return path
}

// writeKeyFile writes a private key file named name to dir.
func writeKeyFile(t *testing.T, dir, name string, block *pem.Block) {
	t.Helper()

	assert.NilError(t, os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(block), 0o600))
}

// writeEd25519Key writes a new ed25519 private key without a passphrase, in PKCS #8 form, to the
// file name in dir.
func writeEd25519Key(t *testing.T, dir, name string) {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	assert.NilError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	assert.NilError(t, err)
	writeKeyFile(t, dir, name, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestAuthCandidatesSSH(t *testing.T) {
	t.Run("no agent and no key files", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("SSH_AUTH_SOCK", "")

		assert.Equal(t, len(authCandidates(newEndpoint(t, "git@github.com:getoutreach/devbase.git"))), 0)
	})

	t.Run("the agent", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("SSH_AUTH_SOCK", listenUnix(t))

		candidates := authCandidates(newEndpoint(t, "ssh://deploy@example.com/repo.git"))
		assert.DeepEqual(t, candidateKinds(candidates), []string{"agent"})
		agentAuth, ok := candidates[0].auth.(*gitssh.PublicKeysCallback)
		assert.Assert(t, ok, "got %T", candidates[0].auth)
		assert.Equal(t, agentAuth.User, "deploy")
	})

	t.Run("an agent that does not answer", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing"))

		assert.Equal(t, len(authCandidates(newEndpoint(t, "git@github.com:getoutreach/devbase.git"))), 0)
	})

	t.Run("key files that exist and load", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("SSH_AUTH_SOCK", "")
		sshDir := filepath.Join(home, ".ssh")
		assert.NilError(t, os.Mkdir(sshDir, 0o700))

		writeEd25519Key(t, sshDir, "id_ed25519")
		// A key that needs a passphrase cannot be loaded.
		writeKeyFile(t, sshDir, "id_rsa", &pem.Block{
			Type: "RSA PRIVATE KEY",
			Headers: map[string]string{
				"Proc-Type": "4,ENCRYPTED",
				"DEK-Info":  "AES-128-CBC,00112233445566778899AABBCCDDEEFF",
			},
			Bytes: make([]byte, 32),
		})
		// A directory cannot be read as a key file.
		assert.NilError(t, os.Mkdir(filepath.Join(sshDir, "id_ecdsa"), 0o700))

		candidates := authCandidates(newEndpoint(t, "git@github.com:getoutreach/devbase.git"))
		assert.DeepEqual(t, candidateKinds(candidates), []string{"key file"})
		keyAuth, ok := candidates[0].auth.(*gitssh.PublicKeys)
		assert.Assert(t, ok, "got %T", candidates[0].auth)
		assert.Equal(t, keyAuth.User, "git")
	})

	t.Run("the agent before the key files", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("SSH_AUTH_SOCK", listenUnix(t))
		sshDir := filepath.Join(home, ".ssh")
		assert.NilError(t, os.Mkdir(sshDir, 0o700))
		writeEd25519Key(t, sshDir, "id_ecdsa")

		candidates := authCandidates(newEndpoint(t, "ssh://example.com/repo.git"))
		assert.DeepEqual(t, candidateKinds(candidates), []string{"agent", "key file"})
	})
}

func TestGoGitRemoteURL(t *testing.T) {
	tests := []struct{ name, url, want string }{
		{name: "https", url: "https://github.com/getoutreach/devbase.git", want: "https://github.com/getoutreach/devbase.git"},
		{name: "http", url: "http://127.0.0.1:1/repo.git", want: "http://127.0.0.1:1/repo.git"},
		{name: "scp-like ssh", url: "git@github.com:getoutreach/devbase.git", want: "git@github.com:getoutreach/devbase.git"},
		{name: "ssh", url: "ssh://git@github.com/getoutreach/devbase.git", want: "ssh://git@github.com/getoutreach/devbase.git"},
		{name: "file", url: "file:///tmp/repo.git"},
		{name: "local path", url: "/tmp/repo.git"},
		{name: "git protocol", url: "git://example.com/repo.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := git.Init(memory.NewStorage(), nil)
			assert.NilError(t, err)
			_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{tt.url}})
			assert.NilError(t, err)

			assert.Equal(t, goGitRemoteURL(repo), tt.want)
		})
	}

	t.Run("no origin", func(t *testing.T) {
		repo, err := git.Init(memory.NewStorage(), nil)
		assert.NilError(t, err)
		assert.Equal(t, goGitRemoteURL(repo), "")
	})
}

// TestGoGitErrorIsNotShown checks that the text of a go-git error, which can contain the remote
// URL with its credentials, is not part of an error.
func TestGoGitErrorIsNotShown(t *testing.T) {
	const url = "http://user:s3cr3t@127.0.0.1:1/x.git"

	_, err := blobViaGoGit(t.Context(), url, plumbing.NewHash(blobHash(baseContent)))
	assert.Assert(t, err != nil)
	assert.Assert(t, !strings.Contains(err.Error(), "s3cr3t"), "got %v", err)

	root, _, baseName := newTestRepo(t)
	clone := newPartialClone(t, root, "blob:none", "feature")
	runGit(t, clone, "remote", "set-url", "origin", url)

	_, err = MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrMissingObject), "got %v", err)
	assert.Assert(t, !strings.Contains(err.Error(), "s3cr3t"), "got %v", err)
}
