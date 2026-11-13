// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Listing the files of a repository with git.

package check

import (
	"context"
	"fmt"
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
