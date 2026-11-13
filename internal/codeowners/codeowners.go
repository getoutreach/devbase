// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Locating and parsing GitHub CODEOWNERS files.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

// Package codeowners finds and parses CODEOWNERS files.
package codeowners

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Sentinel errors returned (wrapped) by Load and LoadFromIndex.
var (
	// ErrNotFound is wrapped by the error that Load and LoadFromIndex return when a repository has
	// no CODEOWNERS file.
	ErrNotFound = errors.New("codeowners file not found")

	// ErrMultipleFiles is wrapped by the error that Load and LoadFromIndex return when a repository
	// has more than one CODEOWNERS file.
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
	rel, content, err := Read(repoDir)
	if err != nil {
		return nil, err
	}

	entries, err := Parse(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(repoDir, rel), err)
	}

	return entries, nil
}

// Read returns rel, the path of the CODEOWNERS file in repoDir, relative to repoDir with "/" as
// the separator, and the content of the file. It looks up the file like Load.
func Read(repoDir string) (rel string, content []byte, err error) {
	var found []string
	for _, loc := range locations() {
		_, err := os.Stat(filepath.Join(repoDir, loc))
		switch {
		case err == nil:
			found = append(found, loc)
		case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
			// Not present at this location.
		default:
			return "", nil, fmt.Errorf("stat %s: %w", loc, err)
		}
	}

	rel, err = selectFile(repoDir, found)
	if err != nil {
		return "", nil, err
	}

	content, err = os.ReadFile(filepath.Join(repoDir, rel))
	if err != nil {
		return "", nil, fmt.Errorf("read CODEOWNERS file: %w", err)
	}

	return rel, content, nil
}

// locations returns the paths where a CODEOWNERS file can be, relative to the root of the
// repository and with "/" as the separator.
func locations() []string {
	return []string{"CODEOWNERS", "docs/CODEOWNERS", ".github/CODEOWNERS"}
}

// selectFile returns the only path in found, which are the locations of the CODEOWNERS files of
// the repository in repoDir. It returns an error when found has no path or more than one.
func selectFile(repoDir string, found []string) (string, error) {
	switch len(found) {
	case 0:
		return "", &loadError{
			msg: "No CODEOWNERS found in the root, docs/, or .github/ directory of the repository " + repoDir,
			err: ErrNotFound,
		}
	case 1:
		return found[0], nil
	default:
		display := make([]string, len(found))
		for i, rel := range found {
			display[i] = "./" + rel
		}

		return "", &loadError{
			msg: fmt.Sprintf("Multiple CODEOWNERS files found in the %s locations of the repository %s",
				oxfordList(display), repoDir),
			err: ErrMultipleFiles,
		}
	}
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
