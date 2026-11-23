// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for how a blob that is missing from a partial clone is fetched
// when the run is canceled or git stalls.

package gitdiff

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"gotest.tools/v3/assert"
)

// TestMergeBaseFilesCanceled checks that a canceled run reports the cancellation, and not that the
// objects are missing and the user needs a full clone.
func TestMergeBaseFilesCanceled(t *testing.T) {
	root, _, baseName := newTestRepo(t)
	clone := newPartialClone(t, root, "blob:none")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := MergeBaseFiles(ctx, clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
	assert.Assert(t, !errors.Is(err, ErrMissingObject), "got %v", err)
}

// TestBlobViaGitDoesNotWaitForChildProcesses checks that a canceled read returns even when a child
// process of git still holds the output of git, as ssh can while it waits for a stalled connection.
func TestBlobViaGitDoesNotWaitForChildProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test replaces git with a shell script")
	}

	// A git that starts a child process in the background, and then stalls with it.
	bin := t.TempDir()
	script := "#!/bin/sh\nsleep 10 &\nsleep 10\n"
	assert.NilError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := blobViaGit(ctx, t.TempDir(), plumbing.NewHash(blobHash(baseContent)))
	assert.Assert(t, err != nil)
	assert.Assert(t, time.Since(start) < 6*time.Second, "blobViaGit waited %s for a child process", time.Since(start))
}
