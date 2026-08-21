// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Discovers *.graphql files and runs Tier 1 spec
// validation against them via gqlparser.

// Package lint runs the Tier 1 rule tier against a repository's
// *.graphql files: 9 rules that gqlparser/v2 enforces for free while
// parsing SDL, needing no custom rule code. FindGraphQLFiles discovers
// the files to lint, respecting scripts/devbase.yaml's exclude
// patterns; Files parses them as one combined schema via
// gqlparser.LoadSchema and turns any resulting parse error into a
// Violation tagged with the Tier 1 rule name it corresponds to, using
// the classification verified in lint_test.go.
//
// gqlparser.LoadSchema stops at the first validation error it finds, so
// Files can only ever report one violation per run; fixing it and
// re-running surfaces the next one, the same behavior a contributor
// would see running gqlparser-based tooling directly.
//
// go.mod pins github.com/vektah/gqlparser/v2 to v2.5.36 (the latest
// v2.5.x release as of 2026-08-21) so this behavior is stable and
// reviewable across releases. gqlparser/v2 also performs a set of
// spec-mandated schema validations beyond the 9 rules classified here
// (for example, rejecting a reserved "__" name prefix, or a zero-field
// object type); those surface as violations tagged UnclassifiedRule.
package lint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// UnclassifiedRule tags a violation that gqlparser raises while parsing
// SDL but that does not correspond to one of the 9 named Tier 1 rules --
// one of gqlparser's other spec-mandated schema validations (for
// example, a reserved "__" name prefix, or a zero-field object type).
const UnclassifiedRule = "gqlparser"

// Violation is a single Tier 1 lint finding, formatted as
// "<file>:<line>:<col>: <message> [<rule>]".
type Violation struct {
	err  *gqlerror.Error
	Rule string
}

// String formats the violation as "<file>:<line>:<col>: <message>
// [<rule>]", using the file name and position gqlparser recorded when
// the error occurred.
func (v Violation) String() string {
	return fmt.Sprintf("%s [%s]", v.err.Error(), v.Rule)
}

// Files parses paths as one combined GraphQL schema and returns the
// Tier 1 violation gqlparser raised, if any. Files are read and parsed
// together, not independently, so that a type defined in one file is
// visible when validating a reference to it in another.
func Files(paths []string) ([]Violation, error) {
	sources := make([]*ast.Source, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		sources = append(sources, &ast.Source{Name: path, Input: string(data)})
	}

	if _, err := gqlparser.LoadSchema(sources...); err != nil {
		var gqlErr *gqlerror.Error
		if !errors.As(err, &gqlErr) {
			return nil, fmt.Errorf("parse graphql schema: %w", err)
		}
		return []Violation{{err: gqlErr, Rule: ruleForMessage(gqlErr.Message)}}, nil
	}

	return nil, nil
}

// ruleForMessage classifies a gqlparser schema-validation error message
// by the Tier 1 rule it corresponds to, using the exact message shapes
// verified in lint_test.go against the gqlparser/v2 version pinned in
// go.mod. A message matching none of them is one of gqlparser's other
// spec validations rather than a named Tier 1 rule.
func ruleForMessage(msg string) string {
	switch {
	case strings.HasPrefix(msg, "Cannot redeclare directive "):
		return config.RuleUniqueDirectiveNames
	case strings.HasPrefix(msg, "Field ") && strings.HasSuffix(msg, "can only be defined once."):
		return config.RuleUniqueFieldDefinitionNames
	case strings.HasPrefix(msg, "Cannot have multiple schema entry points"):
		// gqlparser raises this single message both for a second
		// schema { } block and for a duplicate root operation type
		// declared across two blocks; see the RuleUniqueOperationTypes
		// gap documented in lint_test.go. RuleLoneSchemaDefinition names
		// the check gqlparser actually performs.
		return config.RuleLoneSchemaDefinition
	case strings.HasPrefix(msg, "Cannot redeclare type "):
		return config.RuleUniqueTypeNames
	case strings.HasPrefix(msg, "Undefined argument "):
		return config.RuleKnownArgumentNames
	case strings.HasPrefix(msg, "Undefined directive "):
		return config.RuleKnownDirectives
	case strings.HasPrefix(msg, "Undefined type "):
		return config.RuleKnownTypeNames
	case strings.HasSuffix(msg, "cannot be null."):
		return config.RuleProvidedRequiredArguments
	default:
		return UnclassifiedRule
	}
}

// FindGraphQLFiles walks each of paths, collecting every *.graphql file
// found (in deterministic, lexically sorted order) that does not match
// an exclude pattern. A path naming a file directly is included as-is,
// regardless of extension; a path naming a directory is walked
// recursively. Patterns are matched with doublestar semantics ("**"
// matches zero or more path segments), consistent with the exclude
// examples in scripts/devbase.yaml.
func FindGraphQLFiles(paths, excludes []string) ([]string, error) {
	var files []string
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", root, err)
		}

		if !info.IsDir() {
			files = appendIfIncluded(files, excludes, root)
			continue
		}

		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".graphql" {
				return nil
			}
			files = appendIfIncluded(files, excludes, path)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walk %s: %w", root, walkErr)
		}
	}

	sort.Strings(files)
	return files, nil
}

// appendIfIncluded appends path to files unless it matches one of
// excludes, returning the (possibly unchanged) slice.
func appendIfIncluded(files, excludes []string, path string) []string {
	if matchesAny(excludes, path) {
		return files
	}
	return append(files, path)
}

// matchesAny reports whether path matches any of patterns.
func matchesAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if matchGlob(pattern, path) {
			return true
		}
	}
	return false
}

// matchGlob reports whether path matches pattern, where pattern is a
// slash-separated sequence of segments and a "**" segment matches zero
// or more path segments (doublestar semantics). Every other segment is
// matched against the corresponding path segment with filepath.Match,
// so "*", "?", and character classes work within a single segment but
// never cross a "/".
func matchGlob(pattern, path string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// matchSegments reports whether path's segments match pattern's, per
// the doublestar semantics matchGlob documents.
func matchSegments(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}

	if pattern[0] == "**" {
		if matchSegments(pattern[1:], path) {
			return true
		}
		if len(path) == 0 {
			return false
		}
		return matchSegments(pattern, path[1:])
	}

	if len(path) == 0 {
		return false
	}
	ok, err := filepath.Match(pattern[0], path[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], path[1:])
}
