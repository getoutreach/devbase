// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds CODEOWNERS patterns that match no files in the repository.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// gitlinkMode is the mode of a submodule in the git index.
const gitlinkMode = "160000"

// FileExist validates that every pattern of the CODEOWNERS file matches at least one file of the
// repository. The files are the ones in the git index and, when Input.Staged is not set, the
// untracked files that git does not ignore. A submodule counts as a directory.
type FileExist struct{}

// repoFile is a file of the repository.
type repoFile struct {
	// path is the path of the file, split at "/".
	path []string

	// isDir is true for a submodule.
	isDir bool
}

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

	files, err := repoFiles(ctx, in)
	if err != nil {
		return Output{}, err
	}

	for _, entry := range in.Entries {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}

		// A pattern follows the rules of gitignore files. A negated pattern never excludes a file,
		// so it is reported as matching no files. GitHub does not support negation either.
		pattern := gitignore.ParsePattern(entry.Pattern, nil)

		matched := slices.ContainsFunc(files, func(f repoFile) bool {
			return pattern.Match(f.path, f.isDir) == gitignore.Exclude
		})
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

// repoFiles returns the files in the git index and, when in.Staged is not set, the untracked
// files that git does not ignore.
func repoFiles(ctx context.Context, in Input) ([]repoFile, error) {
	// Each staged entry is "<mode> <object> <stage>\t<path>".
	staged, err := gitLsFiles(ctx, in, "--stage")
	if err != nil {
		return nil, err
	}

	var untracked []string
	if !in.Staged {
		untracked, err = gitLsFiles(ctx, in, "--others", "--exclude-standard")
		if err != nil {
			return nil, err
		}
	}

	files := make([]repoFile, 0, len(staged)+len(untracked))
	for _, entry := range staged {
		_, path, _ := strings.Cut(entry, "\t")
		files = append(files, repoFile{path: strings.Split(path, "/"), isDir: strings.HasPrefix(entry, gitlinkMode+" ")})
	}
	for _, path := range untracked {
		files = append(files, repoFile{path: strings.Split(path, "/")})
	}

	return files, nil
}
