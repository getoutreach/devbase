// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds CODEOWNERS patterns that match no files in the repository.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// FileExist validates that every pattern of the CODEOWNERS file matches at least one file of the
// repository. The files are the ones in the git index and, when Input.Staged is not set, the
// untracked files that git does not ignore.
type FileExist struct{}

// NewFileExist returns a new FileExist instance.
func NewFileExist() *FileExist {
	return &FileExist{}
}

// Check reports the patterns that match no files in the repository.
func (*FileExist) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	args := []string{"--cached"}
	if !in.Staged {
		args = append(args, "--others", "--exclude-standard")
	}

	files, err := gitLsFiles(ctx, in, args...)
	if err != nil {
		return Output{}, err
	}

	paths := make([][]string, len(files))
	for i, file := range files {
		paths[i] = strings.Split(file, "/")
	}

	for _, entry := range in.Entries {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}

		// A pattern follows the rules of gitignore files. A negated pattern never excludes a file,
		// so it is reported as matching no files. GitHub does not support negation either.
		pattern := gitignore.ParsePattern(entry.Pattern, nil)

		matched := false
		for _, path := range paths {
			if pattern.Match(path, false) == gitignore.Exclude {
				matched = true
				break
			}
		}

		if !matched {
			out.Report(fmt.Sprintf("%q does not match any files in repository", entry.Pattern), WithEntry(entry))
		}
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*FileExist) Name() string {
	return "File Exist Checker"
}
