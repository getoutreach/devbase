// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Fetches a blob that is missing from a partial clone with go-git,
// before the git fallback.

package gitdiff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
)

// errGoGitFetch is returned when go-git cannot fetch a blob. The error does not contain the text of
// the go-git errors, because they can contain the remote URL with its credentials.
var errGoGitFetch = errors.New("go-git could not fetch the blob")

// githubHost is the only host that receives the GitHub token.
const githubHost = "github.com"

// The transport protocols, as transport.Endpoint names them, that go-git fetches with.
const (
	// protocolSSH includes the scp-like form user@host:path.
	protocolSSH = "ssh"
	// protocolHTTP sends no credentials from the environment, because they would be in clear text.
	protocolHTTP = "http"
	// protocolHTTPS is the only protocol that can receive the GitHub token.
	protocolHTTPS = "https"
)

// authCandidate is one way to authenticate a go-git fetch. kind names it without secrets, for
// example "agent", "key file", "token" or "none".
type authCandidate struct {
	kind string
	auth transport.AuthMethod
}

// goGitRemote is a remote that go-git fetches blobs from.
type goGitRemote struct {
	url string

	// candidates returns the ways to authenticate. They are resolved at the first call, so a run
	// without a missing blob does not read the key files or connect to the SSH agent.
	candidates func() []authCandidate
}

// parseGoGitRemote returns the remote with the given URL if go-git can fetch from it, that is, if
// it is an ssh, http or https URL. Otherwise it returns nil. go-git runs the git binary for a file
// URL or a local path, so git is used directly for those.
func parseGoGitRemote(rawURL string) *goGitRemote {
	ep, err := transport.NewEndpoint(rawURL)
	if err != nil {
		return nil
	}
	switch ep.Protocol {
	case protocolSSH, protocolHTTP, protocolHTTPS:
		return &goGitRemote{url: rawURL, candidates: sync.OnceValue(func() []authCandidate { return authCandidates(ep) })}
	default:
		return nil
	}
}

// originGoGitRemote returns the origin remote of repo, or nil if go-git does not fetch from it (see
// parseGoGitRemote). go-git applies the url.<base>.insteadOf rules of the configuration of the
// repository, but not those of the global and system configuration, which git applies. A clone that
// relies on those rules can make the go-git fetch fail, and then git reads the blob.
func originGoGitRemote(repo *git.Repository) *goGitRemote {
	cfg, err := repo.Config()
	if err != nil {
		return nil
	}
	origin, ok := cfg.Remotes[git.DefaultRemoteName]
	if !ok || len(origin.URLs) == 0 {
		return nil
	}

	return parseGoGitRemote(origin.URLs[0])
}

// authCandidates returns the ways to authenticate with ep, in the order to try them. For ssh, they
// are the SSH agent and then the default key files without a passphrase. For https to GitHub, it is
// the GitHub token from the environment if there is one. Otherwise it is no authentication.
func authCandidates(ep *transport.Endpoint) []authCandidate {
	if ep.Protocol == protocolSSH {
		return sshAuthCandidates(ep)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("OUTREACH_GITHUB_TOKEN")
	}
	// The token is only sent to GitHub, and never in clear text.
	if token != "" && ep.Protocol == protocolHTTPS && ep.Host == githubHost {
		return []authCandidate{{kind: "token", auth: &githttp.BasicAuth{Username: "x-access-token", Password: token}}}
	}

	return []authCandidate{{kind: "none"}}
}

// sshAuthCandidates returns the SSH agent, if SSH_AUTH_SOCK is set and the agent is reachable, and
// then each default key file that exists and loads without a passphrase.
func sshAuthCandidates(ep *transport.Endpoint) []authCandidate {
	user := ep.User
	if user == "" {
		user = "git"
	}

	var candidates []authCandidate
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		if auth, err := gitssh.NewSSHAgentAuth(user); err == nil {
			candidates = append(candidates, authCandidate{kind: "agent", auth: auth})
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return candidates
	}
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		if auth, err := gitssh.NewPublicKeysFromFile(user, filepath.Join(home, ".ssh", name), ""); err == nil {
			candidates = append(candidates, authCandidate{kind: "key file", auth: auth})
		}
	}

	return candidates
}

// fetch fetches the blob with the given hash from the remote with go-git, and tries each candidate
// of authCandidates until one fetch succeeds. All of the tries share blobFetchTimeout.
func (r *goGitRemote) fetch(ctx context.Context, hash plumbing.Hash) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, blobFetchTimeout)
	defer cancel()

	return fetchBlob(ctx, r.url, hash, r.candidates())
}

// fetchBlob fetches the blob with the given hash from remoteURL with each of candidates in turn,
// and returns the content of the first fetch that succeeds.
func fetchBlob(ctx context.Context, remoteURL string, hash plumbing.Hash, candidates []authCandidate) (string, error) {
	kinds := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if content, err := fetchBlobWith(ctx, remoteURL, hash, c.auth); err == nil {
			return content, nil
		}
		kinds = append(kinds, c.kind)
	}

	if len(kinds) == 0 {
		return "", fmt.Errorf("%w: no authentication to try", errGoGitFetch)
	}

	return "", fmt.Errorf("%w: tried authentication: %s", errGoGitFetch, strings.Join(kinds, ", "))
}

// fetchBlobWith fetches the blob with the given hash from remoteURL into a temporary in-memory
// repository, and returns its content. The blob cannot be fetched into the partial clone: go-git
// tells the server that the clone already has it, so the server does not send it. The in-memory
// repository stores an object under the hash of its content, so a blob that it returns for hash has
// that hash.
func fetchBlobWith(ctx context.Context, remoteURL string, hash plumbing.Hash, auth transport.AuthMethod) (string, error) {
	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		return "", fmt.Errorf("create a repository in memory: %w", err)
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{Name: git.DefaultRemoteName, URLs: []string{remoteURL}})
	if err != nil {
		return "", fmt.Errorf("create the remote: %w", err)
	}
	err = remote.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{config.RefSpec(hash.String() + ":refs/blob")},
		Auth:     auth,
	})
	if err != nil {
		return "", fmt.Errorf("fetch the blob: %w", err)
	}

	blob, err := repo.BlobObject(hash)
	if err != nil {
		return "", fmt.Errorf("read the fetched blob: %w", err)
	}
	r, err := blob.Reader()
	if err != nil {
		return "", fmt.Errorf("read the fetched blob: %w", err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read the fetched blob: %w", err)
	}

	return string(content), nil
}
