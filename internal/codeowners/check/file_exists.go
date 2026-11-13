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
// repository. The files are the ones in the git index. When Input.Staged is not set, they also
// include the untracked files that git does not ignore, and exclude the files deleted from the
// working tree. A submodule counts as a directory.
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

	index := newFileIndex(files)
	for _, entry := range in.Entries {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}

		if !index.match(entry.Pattern) {
			out.Report(fmt.Sprintf("%q does not match any files in repository", entry.Pattern), WithEntry(entry))
		}
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*FileExist) Name() string {
	return "File Exist Checker"
}

// repoFiles returns the files in the git index. When in.Staged is not set, it also returns the
// untracked files that git does not ignore, and leaves out the files deleted from the working tree.
func repoFiles(ctx context.Context, in Input) ([]repoFile, error) {
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

	files := make([]repoFile, 0, len(staged)+len(untracked))
	for _, entry := range staged {
		_, path, _ := strings.Cut(entry, "\t")
		if !gone[path] {
			files = append(files, repoFile{path: strings.Split(path, "/"), isDir: strings.HasPrefix(entry, gitlinkMode+" ")})
		}
	}
	for _, path := range untracked {
		files = append(files, repoFile{path: strings.Split(path, "/")})
	}

	return files, nil
}

// fileIndex is the files of the repository, also grouped by the first element of their path.
type fileIndex struct {
	// files are all the files.
	files []repoFile

	// byTop are the files grouped by the first element of their path.
	byTop map[string][]repoFile
}

// newFileIndex returns the index of files.
func newFileIndex(files []repoFile) *fileIndex {
	byTop := make(map[string][]repoFile)
	for _, f := range files {
		byTop[f.path[0]] = append(byTop[f.path[0]], f)
	}

	return &fileIndex{files: files, byTop: byTop}
}

// match reports whether the CODEOWNERS pattern matches a file. For a pattern that anchoredTop
// accepts, it only tries the files below the first element of the pattern.
func (x *fileIndex) match(pattern string) bool {
	// A pattern follows the rules of gitignore files. A negated pattern never excludes a file,
	// so it matches no files. GitHub does not support negation either.
	p := gitignore.ParsePattern(pattern, nil)

	candidates := x.files
	if top, ok := anchoredTop(pattern); ok {
		candidates = x.byTop[top]
	}

	return slices.ContainsFunc(candidates, func(f repoFile) bool {
		return p.Match(f.path, f.isDir) == gitignore.Exclude
	})
}

// anchoredTop returns the first element of an anchored pattern, and false when the pattern is not
// anchored or the element has glob characters. Like gitignore.ParsePattern, it ignores a leading
// "!", trailing spaces, and a trailing "/". A pattern that still has a "/" is anchored, and its
// first non-empty element only matches the first element of a path.
func anchoredTop(pattern string) (string, bool) {
	p := strings.TrimPrefix(pattern, "!")
	if !strings.HasSuffix(p, `\ `) {
		p = strings.TrimRight(p, " ")
	}
	p = strings.TrimSuffix(p, "/")

	if !strings.Contains(p, "/") {
		return "", false
	}

	for elem := range strings.SplitSeq(p, "/") {
		if elem == "" {
			continue
		}
		if strings.ContainsAny(elem, `*?[\`) {
			return "", false
		}

		return elem, true
	}

	return "", false
}
