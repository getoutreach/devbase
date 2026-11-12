// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Path globs of release units: which changed files belong
// to a unit or to the root release.

package spec

import (
	"fmt"
	"path"
	"strings"
)

// ValidatePathGlob checks a path glob: a path relative to the
// repository, with "/" separators, whose segments are path.Match
// patterns, or "**" for any number of segments. It has no leading "/" or
// "./", no "." or ".." segment, no "\", and no "!" negation.
func ValidatePathGlob(g string) error {
	if g == "" || strings.HasPrefix(g, "/") || strings.HasPrefix(g, "!") || strings.Contains(g, `\`) {
		return fmt.Errorf("%w: path glob %q", ErrUnsupported, g)
	}
	for seg := range strings.SplitSeq(g, "/") {
		switch {
		case seg == "**":
		case seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "**"):
			return fmt.Errorf("%w: path glob %q", ErrUnsupported, g)
		default:
			if _, err := path.Match(seg, ""); err != nil {
				return fmt.Errorf("%w: path glob %q: %w", ErrUnsupported, g, err)
			}
		}
	}
	return nil
}

// MatchPath reports whether file, a path relative to the repository,
// matches the path glob g, or is in a folder that g matches. So
// "node/widgets" and "node/widgets/**" both match "node/widgets/index.ts".
// A glob is anchored at the repository root: "*.md" matches only files
// there.
func MatchPath(g, file string) bool {
	return matchSegments(strings.Split(g, "/"), strings.Split(file, "/"))
}

// matchSegments matches the glob segments p with the path segments f.
func matchSegments(p, f []string) bool {
	if len(p) == 0 {
		return true // The glob matched a folder of the file, or the file.
	}
	if p[0] == "**" {
		for i := 0; i <= len(f); i++ {
			if matchSegments(p[1:], f[i:]) {
				return true
			}
		}
		return false
	}
	if len(f) == 0 {
		return false
	}
	if ok, err := path.Match(p[0], f[0]); err != nil || !ok {
		return false
	}
	return matchSegments(p[1:], f[1:])
}

// MatchAnyPath reports whether file matches one of the path globs.
func MatchAnyPath(globs []string, file string) bool {
	for _, g := range globs {
		if MatchPath(g, file) {
			return true
		}
	}
	return false
}
