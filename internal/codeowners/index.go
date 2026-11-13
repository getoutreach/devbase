// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loading the CODEOWNERS file from the git index.

package codeowners

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/codeowners/gitcmd"
)

// LoadFromIndex returns the entries of the CODEOWNERS file in the git index of the repository in
// repoDir, so changes that are not staged are ignored. The file is looked up in the same
// locations as Load. With trust, git treats repoDir as a safe directory, even when another user
// owns it.
func LoadFromIndex(ctx context.Context, repoDir string, trust bool) ([]Entry, error) {
	listed, err := gitcmd.Run(ctx, repoDir, trust, append([]string{"ls-files", "-z", "--cached", "--"}, locations()...)...)
	if err != nil {
		return nil, fmt.Errorf("list the CODEOWNERS files in the git index: %w", err)
	}

	// A pathspec also matches the files below a directory, so only exact paths count.
	staged := strings.Split(string(listed), "\x00")
	found := slices.DeleteFunc(locations(), func(rel string) bool { return !slices.Contains(staged, rel) })

	rel, err := selectFile(repoDir, found)
	if err != nil {
		return nil, err
	}

	// The listed paths are relative to repoDir. Without "./", git reads a path in the index
	// relative to the root of the work tree.
	content, err := gitcmd.Run(ctx, repoDir, trust, "cat-file", "blob", ":./"+rel)
	if err != nil {
		return nil, fmt.Errorf("read %s from the git index: %w", rel, err)
	}

	entries, err := Parse(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse %s from the git index: %w", rel, err)
	}

	return entries, nil
}
