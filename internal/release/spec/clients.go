// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Client version files: the package manifests that the
// client publishers set only in the release checkout, so that the
// release commit never has them.

package spec

import (
	"fmt"
	"path"
	"slices"
)

// npmVersionFiles are the files in pkgRoot that npm version changes.
//
//nolint:gochecknoglobals // Why: read-only list.
var npmVersionFiles = []string{"package.json", "package-lock.json", "npm-shrinkwrap.json"}

// ClientVersionFiles returns the path.Match patterns, relative to the
// repository, of the client version files: the files that a client
// publisher sets only in the release checkout (requirement 12).
func (c *Config) ClientVersionFiles() []string {
	var files []string
	for _, p := range c.Plugins {
		if p.NPM != nil && !path.IsAbs(p.NPM.PkgRoot) {
			for _, f := range npmVersionFiles {
				files = append(files, path.Join(p.NPM.PkgRoot, f))
			}
		}
	}
	return files
}

// IsClientVersionFile reports whether file, a slash-separated path
// relative to the repository, is a client version file.
func (c *Config) IsClientVersionFile(file string) bool {
	file = path.Clean(file)
	return slices.ContainsFunc(c.ClientVersionFiles(), func(p string) bool { return globsOverlap(p, file) })
}

// DropClientVersionFiles removes the release commit globs that name a
// client version file, and returns a warning for each. The release
// commit also leaves out a client version file that another glob, such
// as a folder, matches.
func (c *Config) DropClientVersionFiles() []string {
	var warnings []string
	files := c.ClientVersionFiles()
	for _, p := range c.Plugins {
		if p.Commit == nil || p.Commit.Files == nil {
			continue
		}
		// A non-nil empty list commits nothing, while nil commits the
		// default files.
		p.Commit.Files = slices.DeleteFunc(p.Commit.Files, func(g string) bool {
			clean := path.Clean(g)
			if !slices.ContainsFunc(files, func(f string) bool { return globsOverlap(clean, f) }) {
				return false
			}
			warnings = append(warnings, fmt.Sprintf(
				"the release does not commit the client version file %q; remove it from the release commit files", g))
			return true
		})
	}
	return warnings
}

// globsOverlap reports whether the path.Match patterns a and b are the
// same, or one matches the other as a path, which is true for a file
// name that matches a pattern.
func globsOverlap(a, b string) bool {
	ab, errA := path.Match(a, b)
	ba, errB := path.Match(b, a)
	return a == b || (errA == nil && ab) || (errB == nil && ba)
}
