// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds files in the repository that no CODEOWNERS pattern matches.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
)

// NotOwnedFileConfig configures the NotOwnedFile check.
type NotOwnedFileConfig struct {
	// SkipPatterns are CODEOWNERS patterns that the check ignores, such as a default "*" pattern.
	// A file that only these patterns match is reported as not owned.
	SkipPatterns []string

	// Subdirectories limits the check to the files in these directories. Each one is a git
	// pathspec, and Check returns an error when one matches no file in the git index. All the
	// files of the repository are checked when it is empty.
	Subdirectories []string
}

// NotOwnedFile validates that every file that git tracks has an owner, like GitHub: the last entry
// of the CODEOWNERS file whose pattern matches the file must have owners. A submodule counts as a
// directory. It only reads the git index, so it works on a repository that has uncommitted changes
// and it never changes the repository.
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

// Check reports the files that have no owner, because no pattern of the CODEOWNERS file matches
// them, or because the last pattern that matches them has no owners.
func (c *NotOwnedFile) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	if len(in.Entries) == 0 {
		out.Report("The CODEOWNERS file is empty. The files in the repository don't have any owner.")
		return out, nil
	}

	files, err := indexPaths(ctx, in, append([]string{"--error-unmatch", "--"}, c.subdirectories...)...)
	if err != nil {
		return Output{}, err
	}

	matcher := codeowners.NewMatcher(slices.DeleteFunc(slices.Clone(in.Entries), func(entry codeowners.Entry) bool {
		return slices.Contains(c.skipPatterns, entry.Pattern)
	}))

	notOwned := slices.DeleteFunc(files, func(file string) bool {
		entry, ok := matcher.Match(file)
		return ok && len(entry.Owners) > 0
	})

	if len(notOwned) > 0 {
		// The path of a submodule is shown without the "/" that makes patterns match it as a directory.
		list := bulletList(notOwned, func(file string) string { return strings.TrimSuffix(file, "/") })
		out.Report(fmt.Sprintf("Found %d not owned files (skipped patterns: %q):\n%s",
			len(notOwned), strings.Join(c.skipPatterns, ","), list))
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*NotOwnedFile) Name() string {
	return "[Experimental] Not Owned File Checker"
}
