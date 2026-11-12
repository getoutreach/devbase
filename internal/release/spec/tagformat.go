// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tag formats: the git tag of a version, such as
// "v${version}" or "api/v${version}".

package spec

import (
	"fmt"
	"strings"
)

// versionVar is the placeholder of the version in a tag format.
const versionVar = "${version}"

// TagFormat is the template of a release tag, with ${version} exactly
// once, as the tagFormat of semantic-release.
type TagFormat string

// RootTagFormat is the tag format of the root release.
const RootTagFormat TagFormat = "v" + versionVar

// Render returns the tag of version.
func (f TagFormat) Render(version string) string {
	return strings.Replace(string(f), versionVar, version, 1)
}

// Prefix returns the text before ${version}, which every tag of the
// format starts with.
func (f TagFormat) Prefix() string {
	prefix, _, _ := strings.Cut(string(f), versionVar)
	return prefix
}

// Version returns the text in the place of ${version} in tag, and false
// when tag does not have the prefix and the suffix of the format.
func (f TagFormat) Version(tag string) (string, bool) {
	prefix, suffix, _ := strings.Cut(string(f), versionVar)
	if len(tag) < len(prefix)+len(suffix) || !strings.HasPrefix(tag, prefix) || !strings.HasSuffix(tag, suffix) {
		return "", false
	}
	return tag[len(prefix) : len(tag)-len(suffix)], true
}

// Validate checks a unit tag format: ${version} exactly once, after a
// prefix that is not empty, and a valid git tag name, as semantic-release
// checks its tagFormat. The prefix must not be one that the root format
// "v${version}" also reads, and must not hold glob characters, because
// the tag list uses it as a pattern.
func (f TagFormat) Validate() error {
	s := string(f)
	prefix := f.Prefix()
	switch {
	case strings.Count(s, versionVar) != 1:
		return fmt.Errorf("%w: tag format %q must contain %s exactly once", ErrUnsupported, s, versionVar)
	case prefix == "":
		return fmt.Errorf("%w: tag format %q must start with a prefix", ErrUnsupported, s)
	case prefix[0] == 'v' && (len(prefix) == 1 || prefix[1] >= '0' && prefix[1] <= '9' || prefix[1] == '='):
		return fmt.Errorf("%w: tag format %q looks like a root tag %q", ErrUnsupported, s, RootTagFormat)
	case strings.ContainsAny(strings.Replace(s, versionVar, "", 1), "${}"):
		return fmt.Errorf("%w: tag format %q has another template", ErrUnsupported, s)
	case !validBranchName("refs/tags/" + f.Render("1.0.0")):
		return fmt.Errorf("%w: tag format %q does not make a valid git tag", ErrUnsupported, s)
	}
	return nil
}
