// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The rules and defaults of the Ruby gem plugin.

package spec

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// gemNameRegex matches a gem name, as Gem::Specification#validate_name
// allows it.
var gemNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validate checks that the folders are clean paths in the repository,
// and that the gem name is valid.
func (g *GemPlugin) validate() error {
	for _, f := range []struct{ name, value string }{{"dir", g.Dir}, {"version file", g.VersionFile}} {
		if f.value != "" && !isLocalPath(f.value) {
			return fmt.Errorf("%w: gem %s %q is not a clean relative path", ErrUnsupported, f.name, f.value)
		}
	}
	switch {
	case g.VersionFile != "" && !strings.HasSuffix(g.VersionFile, ".rb"):
		return fmt.Errorf("%w: gem version file %q is not a Ruby file", ErrUnsupported, g.VersionFile)
	case g.Name != "" && !gemNameRegex.MatchString(g.Name):
		return fmt.Errorf("%w: gem name %q", ErrUnsupported, g.Name)
	}
	return nil
}

// isLocalPath reports whether p is a clean, relative, slash-separated
// path that stays in the repository.
func isLocalPath(p string) bool {
	return !strings.Contains(p, `\`) && path.Clean(p) == p && filepath.IsLocal(p)
}

// GemDir returns the gem folder of g, relative to the repository.
func (g *GemPlugin) GemDir() string {
	if g.Dir == "" {
		return DefaultGemDir
	}
	return g.Dir
}

// VersionPath returns the version file of the gem named name, relative
// to the repository: VersionFile, or "lib/<name>/version.rb", in the gem
// folder.
func (g *GemPlugin) VersionPath(name string) string {
	file := g.VersionFile
	if file == "" {
		file = path.Join("lib", name, "version.rb")
	}
	return path.Join(g.GemDir(), file)
}
