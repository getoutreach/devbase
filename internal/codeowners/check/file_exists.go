// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds CODEOWNERS patterns that match no files in the repository.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
)

// gitlinkMode is the mode of a submodule in the git index.
const gitlinkMode = "160000"

// FileExist validates that every pattern of the CODEOWNERS file matches at least one file of the
// repository, with the rules of codeowners.Pattern. The files are the ones in the git index. When
// Input.Staged is not set, they also include the untracked files that git does not ignore, and
// exclude the files deleted from the working tree. A submodule counts as a directory.
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

	paths, err := repoPaths(ctx, in)
	if err != nil {
		return Output{}, err
	}

	index := newFileIndex(paths)
	for _, entry := range in.Entries {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}

		if !index.match(codeowners.CompilePattern(entry.Pattern)) {
			out.Report(fmt.Sprintf("%q does not match any files in repository", entry.Pattern), WithEntry(entry))
		}
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*FileExist) Name() string {
	return "File Exist Checker"
}

// repoPaths returns the paths of the files in the git index. When in.Staged is not set, it also
// returns the untracked files that git does not ignore, and leaves out the files deleted from the
// working tree. The path of a submodule ends with "/", so that patterns match it as a directory.
func repoPaths(ctx context.Context, in Input) ([]string, error) {
	// Each staged entry is "<mode> <object> <stage>\t<path>".
	staged, err := gitLsFiles(ctx, in, "--stage")
	if err != nil {
		return nil, err
	}

	var untracked, deleted []string
	if !in.Staged {
		if untracked, err = gitLsFiles(ctx, in, "--others", "--exclude-standard"); err != nil {
			return nil, err
		}
		if deleted, err = gitLsFiles(ctx, in, "--deleted"); err != nil {
			return nil, err
		}
	}

	gone := make(map[string]bool, len(deleted))
	for _, path := range deleted {
		gone[path] = true
	}

	paths := make([]string, 0, len(staged)+len(untracked))
	for _, entry := range staged {
		_, path, _ := strings.Cut(entry, "\t")
		if gone[path] {
			continue
		}
		if strings.HasPrefix(entry, gitlinkMode+" ") {
			path += "/"
		}
		paths = append(paths, path)
	}

	return append(paths, untracked...), nil
}

// fileIndex is the paths of the files of the repository, also grouped by their first segment.
type fileIndex struct {
	// paths are the paths of all the files.
	paths []string

	// byFirst are the paths grouped by their first segment.
	byFirst map[string][]string
}

// newFileIndex returns the index of the paths.
func newFileIndex(paths []string) *fileIndex {
	byFirst := make(map[string][]string)
	for _, path := range paths {
		first, _, _ := strings.Cut(path, "/")
		byFirst[first] = append(byFirst[first], path)
	}

	return &fileIndex{paths: paths, byFirst: byFirst}
}

// match reports whether the pattern matches a file. When the pattern only matches paths that
// start with one segment, it only tries the paths that start with that segment.
func (x *fileIndex) match(p codeowners.Pattern) bool {
	candidates := x.paths
	if first, ok := p.FirstSegment(); ok {
		candidates = x.byFirst[first]
	}

	return slices.ContainsFunc(candidates, p.Match)
}
