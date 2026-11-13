// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds files in the repository that no CODEOWNERS pattern matches.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
)

// NotOwnedFileConfig configures the NotOwnedFile check.
type NotOwnedFileConfig struct {
	// SkipPatterns are CODEOWNERS patterns that do not count as owning a file. A file that only
	// these patterns match is reported as not owned.
	SkipPatterns []string

	// Subdirectories limits the check to the files in these directories. Each one is a git
	// pathspec, and Check returns an error when one matches no file in the git index. All the
	// files of the repository are checked when it is empty.
	Subdirectories []string
}

// NotOwnedFile validates that every file that git tracks has an owner. It only reads the git
// index, so it works on a repository that has uncommitted changes and it never changes the
// repository.
type NotOwnedFile struct {
	skipPatterns   []string
	subdirectories []string
}

// NewNotOwnedFile returns a new NotOwnedFile instance.
func NewNotOwnedFile(cfg NotOwnedFileConfig) *NotOwnedFile {
	skip := slices.Clone(cfg.SkipPatterns)
	slices.Sort(skip)

	return &NotOwnedFile{
		skipPatterns:   slices.Compact(skip),
		subdirectories: cfg.Subdirectories,
	}
}

// Check reports the files that no pattern of the CODEOWNERS file matches.
func (c *NotOwnedFile) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	if len(in.Entries) == 0 {
		out.Report("The CODEOWNERS file is empty. The files in the repository don't have any owner.")
		return out, nil
	}

	owned, err := c.ownedFiles(ctx, in)
	if err != nil {
		return Output{}, err
	}

	all, err := gitLsFiles(ctx, in, c.withSubdirectories("--cached", "--error-unmatch")...)
	if err != nil {
		return Output{}, err
	}

	ownedSet := make(map[string]struct{}, len(owned))
	for _, file := range owned {
		ownedSet[file] = struct{}{}
	}

	// git lists the files in index order, so the stages of a file in a merge conflict are adjacent.
	notOwned := slices.DeleteFunc(slices.Compact(all), func(file string) bool {
		_, ok := ownedSet[file]
		return ok
	})

	if len(notOwned) > 0 {
		out.Report(fmt.Sprintf("Found %d not owned files (skipped patterns: %q):\n%s",
			len(notOwned), strings.Join(c.skipPatterns, ","), listFormat(notOwned)))
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*NotOwnedFile) Name() string {
	return "[Experimental] Not Owned File Checker"
}

// withSubdirectories returns args followed by the subdirectories to limit the listing to.
func (c *NotOwnedFile) withSubdirectories(args ...string) []string {
	return append(append(args, "--"), c.subdirectories...)
}

// ownedFiles returns the files in the git index that a pattern of the CODEOWNERS file matches.
// The patterns go to git in a temporary file, so the repository stays as it is.
func (c *NotOwnedFile) ownedFiles(ctx context.Context, in Input) (owned []string, err error) {
	var patterns strings.Builder

	for _, entry := range in.Entries {
		if slices.Contains(c.skipPatterns, entry.Pattern) || strings.HasPrefix(entry.Pattern, "!") {
			continue
		}

		patterns.WriteString(entry.Pattern + "\n")
	}

	f, err := os.CreateTemp("", "codeowners-patterns-*")
	if err != nil {
		return nil, fmt.Errorf("create pattern file: %w", err)
	}

	defer func() {
		// The file is in the temporary directory, so a file that is already gone is not a problem.
		if rmErr := os.Remove(f.Name()); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove pattern file: %w", rmErr))
		}
	}()

	_, writeErr := f.WriteString(patterns.String())
	if closeErr := errors.Join(writeErr, f.Close()); closeErr != nil {
		return nil, fmt.Errorf("write pattern file: %w", closeErr)
	}

	return gitLsFiles(ctx, in, c.withSubdirectories("--cached", "--ignored", "--exclude-from="+f.Name())...)
}

// listFormat formats the lines as a bullet point list.
func listFormat(lines []string) string {
	points := make([]string, len(lines))
	for i, line := range lines {
		points[i] = "            * " + line
	}

	return strings.Join(points, "\n")
}
