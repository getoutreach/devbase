// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Chooses how to fetch a blob that is missing from a partial clone: go-git
// (see gofetch.go) first, then git.

package gitdiff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

// blobFetchTimeout limits how long the go-git fetch, and separately the git fallback, may each take
// to fetch one missing blob, so that a stalled connection cannot hang a CI job.
const blobFetchTimeout = 2 * time.Minute

// blobFetcher fetches blobs that are missing from the repository at root, as in a partial clone.
type blobFetcher struct {
	// root is the root of the working tree of the repository.
	root string
	// remote is the remote that go-git fetches from. It is nil when go-git is not used.
	remote *goGitRemote
	// goGitFailed is true after a go-git fetch failed.
	goGitFailed bool
}

// fetch returns the content of the blob with the given hash. It tries go-git first, and git if
// go-git fails. The go-git error is discarded, because it can contain the remote URL with its
// credentials. After a go-git failure, the next blobs are read with git only, because go-git
// would most likely fail again for the same reason.
func (f *blobFetcher) fetch(ctx context.Context, hash plumbing.Hash) (string, error) {
	if f.remote != nil && !f.goGitFailed {
		content, err := f.remote.fetch(ctx, hash)
		if err == nil {
			return content, nil
		}
		f.goGitFailed = true
	}

	return blobViaGit(ctx, f.root, hash)
}

// blobViaGit reads the blob with the given hash from the repository at root with `git cat-file`.
// It is the fallback when go-git cannot fetch the blob. In a partial clone, git fetches a missing
// blob from the remote that the clone came from, with the credentials and configuration of the
// clone. The git output is not part of the error, because a failed fetch can print the URL of the
// remote with its credentials.
func blobViaGit(ctx context.Context, root string, hash plumbing.Hash) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, blobFetchTimeout)
	defer cancel()

	var stdout bytes.Buffer
	//nolint:gosec // Why: The command is always git cat-file, and the argument is a hash.
	cmd := exec.CommandContext(ctx, "git", "-C", root, "cat-file", "blob", hash.String())
	// A fetch that needs credentials must fail, not wait for a password on the terminal.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: git could not read blob %s: %w", ErrMissingObject, hash, err)
	}

	return stdout.String(), nil
}
