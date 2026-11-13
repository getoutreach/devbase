// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Locating and parsing GitHub CODEOWNERS files.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

// Package codeowners finds and parses CODEOWNERS files.
package codeowners

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Sentinel errors returned (wrapped) by Load.
var (
	// ErrNotFound is wrapped by the error that Load returns when a repository has no CODEOWNERS file.
	ErrNotFound = errors.New("codeowners file not found")

	// ErrMultipleFiles is wrapped by the error that Load returns when a repository has more than one CODEOWNERS file.
	ErrMultipleFiles = errors.New("multiple codeowners files found")
)

// loadError is an error with a message that differs from the sentinel error that it wraps.
type loadError struct {
	// msg is the text returned by Error.
	msg string

	// err is the sentinel error returned by Unwrap.
	err error
}

// Error implements the error interface.
func (e *loadError) Error() string { return e.msg }

// Unwrap returns the sentinel error, so that errors.Is can match it.
func (e *loadError) Unwrap() error { return e.err }

// Entry contains owners for a given pattern.
type Entry struct {
	// LineNo is the 1-based line number of the entry in the CODEOWNERS file.
	LineNo uint64

	// Pattern is the file pattern.
	Pattern string

	// Owners are the owners of files that match Pattern.
	Owners []string
}

// String returns a human-readable form of the entry.
func (e Entry) String() string {
	return fmt.Sprintf("line %d: %s\t%v", e.LineNo, e.Pattern, strings.Join(e.Owners, ", "))
}

// Load returns the entries of the CODEOWNERS file in repoDir. The file is looked up in the
// root, docs/, and .github/ directories.
// See: https://help.github.com/articles/about-code-owners/#codeowners-file-location
func Load(repoDir string) ([]Entry, error) {
	var found []string
	for _, dir := range []string{".", "docs", ".github"} {
		rel := filepath.Join(dir, "CODEOWNERS")
		_, err := os.Stat(filepath.Join(repoDir, rel))
		switch {
		case err == nil:
			found = append(found, rel)
		case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
			// Not present at this location.
		default:
			return nil, fmt.Errorf("stat %s: %w", rel, err)
		}
	}

	switch len(found) {
	case 0:
		return nil, &loadError{
			msg: "No CODEOWNERS found in the root, docs/, or .github/ directory of the repository " + repoDir,
			err: ErrNotFound,
		}
	case 1:
		return loadFile(filepath.Join(repoDir, found[0]))
	default:
		display := make([]string, len(found))
		for i, rel := range found {
			display[i] = "./" + filepath.ToSlash(rel)
		}

		return nil, &loadError{
			msg: fmt.Sprintf("Multiple CODEOWNERS files found in the %s locations of the repository %s",
				oxfordList(display), repoDir),
			err: ErrMultipleFiles,
		}
	}
}

// loadFile parses the CODEOWNERS file at path.
func loadFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CODEOWNERS file: %w", err)
	}
	defer f.Close()

	entries, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return entries, nil
}

// oxfordList joins at least two items as an English list with an Oxford comma,
// e.g. "a and b" or "a, b, and c".
func oxfordList(items []string) string {
	switch len(items) {
	case 2: //nolint:mnd // Why: two items are joined without a comma.
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

// Parse returns the entries of a CODEOWNERS file. A UTF-8 byte order mark at the start, blank
// lines, and comment lines are skipped, and the owners of an entry end at the first inline comment.
func Parse(r io.Reader) ([]Entry, error) {
	var entries []Entry
	s := bufio.NewScanner(r)
	for no := uint64(1); s.Scan(); no++ {
		line := s.Text()
		if no == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}

		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") { // empty or comment
			continue
		}

		owners := fields[1:]
		if i := slices.IndexFunc(owners, func(f string) bool { return strings.HasPrefix(f, "#") }); i >= 0 {
			owners = owners[:i]
		}

		entries = append(entries, Entry{Pattern: fields[0], Owners: owners, LineNo: no})
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scan CODEOWNERS: %w", err)
	}

	return entries, nil
}
