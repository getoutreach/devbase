// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loading the CODEOWNERS file from the git index.

package codeowners

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// LoadFromIndex returns the entries of the CODEOWNERS file in the git index of the repository in
// repoDir, so changes that are not staged are ignored. The file is looked up in the same
// locations as Load. With trust, git treats repoDir as a safe directory, even when another user
// owns it.
func LoadFromIndex(ctx context.Context, repoDir string, trust bool) ([]Entry, error) {
	listed, err := git(ctx, repoDir, trust, append([]string{"ls-files", "-z", "--cached", "--"}, locations()...)...)
	if err != nil {
		return nil, err
	}

	// A pathspec also matches the files below a directory, so only exact paths count.
	staged := strings.Split(string(listed), "\x00")
	found := slices.DeleteFunc(locations(), func(rel string) bool { return !slices.Contains(staged, rel) })

	rel, err := selectFile(repoDir, found)
	if err != nil {
		return nil, err
	}

	content, err := git(ctx, repoDir, trust, "cat-file", "blob", ":"+rel)
	if err != nil {
		return nil, err
	}

	entries, err := Parse(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse %s from the git index: %w", rel, err)
	}

	return entries, nil
}

// git runs git with args in dir and returns its standard output. With trust, git treats dir as a
// safe directory.
func git(ctx context.Context, dir string, trust bool, args ...string) ([]byte, error) {
	if trust {
		args = append([]string{"-c", "safe.directory=" + dir}, args...)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}
