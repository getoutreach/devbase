// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Listing the files of a repository with git.

package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/codeowners/gitcmd"
)

// gitLsFiles runs "git ls-files -z" with args in the repository directory and returns the listed
// paths, which are relative to the directory and use "/" as the separator.
func gitLsFiles(ctx context.Context, in Input, args ...string) ([]string, error) {
	out, err := gitcmd.Run(ctx, in.RepoDir, in.TrustRepoDir, append([]string{"ls-files", "-z"}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list the files of the repository: %w", err)
	}

	return strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 }), nil
}

// indexPaths returns the paths of the entries of the git index that "git ls-files --stage" lists
// with args. The paths are relative to the repository directory and use "/" as the separator. The
// path of a submodule ends with "/", so that patterns match it as a directory. A file in a merge
// conflict is listed once.
func indexPaths(ctx context.Context, in Input, args ...string) ([]string, error) {
	entries, err := gitcmd.ListIndex(ctx, in.RepoDir, in.TrustRepoDir, args...)
	if err != nil {
		return nil, fmt.Errorf("list the files of the repository: %w", err)
	}

	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
		if entry.IsSubmodule() {
			paths[i] += "/"
		}
	}

	// git lists the entries in index order, so the stages of a file in a merge conflict are adjacent.
	return slices.Compact(paths), nil
}
