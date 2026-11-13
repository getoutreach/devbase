// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Listing the files of a repository with git.

package check

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// gitArgs returns the arguments of the git command that lists files.
func gitArgs(in Input, args ...string) []string {
	var full []string
	if in.TrustRepoDir {
		full = append(full, "-c", "safe.directory="+in.RepoDir)
	}

	full = append(full, "ls-files", "-z")

	return append(full, args...)
}

// gitLsFiles runs "git ls-files -z" with args in the repository directory and returns the listed
// paths, which are relative to the directory and use "/" as the separator.
func gitLsFiles(ctx context.Context, in Input, args ...string) ([]string, error) {
	var stdout, stderr bytes.Buffer

	//nolint:gosec // Why: The command is always git ls-files, with arguments from this package.
	cmd := exec.CommandContext(ctx, "git", gitArgs(in, args...)...)
	cmd.Dir = in.RepoDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list files of %s: %w: %s", in.RepoDir, err, strings.TrimSpace(stderr.String()))
	}

	return strings.FieldsFunc(stdout.String(), func(r rune) bool { return r == 0 }), nil
}
