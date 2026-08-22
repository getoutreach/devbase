// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Discovers *.graphql files and runs Tier 1 spec
// validation against them via gqlparser.

// Package lint runs the Tier 1, Tier 2, and (so far, partially) Tier 3
// rule tiers against a repository's *.graphql files. Tier 1 is 10
// rules that gqlparser/v2 enforces for free while parsing SDL, needing
// no custom rule code. FindGraphQLFiles discovers the files to lint,
// respecting scripts/devbase.yaml's exclude patterns; Files parses
// them as one combined schema and turns any resulting parse error into
// a Violation tagged with the Tier 1 rule name it corresponds to,
// using the classification verified in lint_test.go.
//
// Schema validation stops at the first Tier 1 error it finds, so Files
// can only ever report one Tier 1 violation per run; fixing it and
// re-running surfaces the next one, the same behavior a contributor
// would see running gqlparser-based tooling directly. Once a schema
// validates cleanly, Files runs the Tier 2 gap-fill passes
// (directives.go) and the Tier 3 custom rules implemented so far
// (descriptions.go) against it and can report any number of their
// violations in one run, since those are ordinary Go code walking the
// parsed schema rather than a stop-at-first-error parser.
//
// federation.go is the one exception to "no custom rule code" for Tier
// 1: Apollo Federation directives and repo-specific custom scalars are
// never declared via SDL that a subgraph owns, so scripts/devbase.yaml's
// federation and scalars settings tell Files what to synthesize and
// merge in before validation, instead of gqlparser rejecting them as
// undefined.
//
// go.mod pins github.com/vektah/gqlparser/v2 to v2.5.36 (the latest
// v2.5.x release as of 2026-08-21) so this behavior is stable and
// reviewable across releases. gqlparser/v2 also performs a set of
// spec-mandated schema validations beyond the 10 rules classified here
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
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

// UnclassifiedRule tags a violation that gqlparser raises while parsing
// SDL but that does not correspond to one of the 10 named Tier 1 rules --
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

// Files parses paths as one combined GraphQL schema, returning the
// Tier 1 violation gqlparser raised, if any, or -- once the schema
// validates cleanly -- any Tier 2 gap-fill violations found in it
// (directives.go). Files are read and parsed together, not
// independently, so that a type defined in one file is visible when
// validating a reference to it in another.
//
// cfg supplies scripts/devbase.yaml's federation and scalars settings,
// merged into the parsed sources before validation; a nil cfg parses
// paths exactly as written, with no merged prelude.
func Files(paths []string, cfg *config.Lint) ([]Violation, error) {
	fileSources := make([]*ast.Source, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		fileSources = append(fileSources, &ast.Source{Name: path, Input: string(data)})
	}

	extraSources, err := preludeSources(fileSources, cfg)
	if err != nil {
		// preludeSources parses paths' own files looking for @link, so a
		// plain syntax error can surface here instead of from
		// parseAndValidate below. Classify it into a Violation the same
		// way, so syntax errors read consistently regardless of the
		// federation setting. A federation- or scalars-specific error
		// carries a sentinel in gqlErr.Err and is returned as-is.
		var gqlErr *gqlerror.Error
		if errors.As(err, &gqlErr) && gqlErr.Err == nil {
			return []Violation{{err: gqlErr, Rule: ruleForMessage(gqlErr.Message)}}, nil
		}
		return nil, err
	}

	sources := make([]*ast.Source, 0, len(fileSources)+len(extraSources))
	sources = append(sources, fileSources...)
	sources = append(sources, extraSources...)

	parsed, tier1Violation, err := parseAndValidate(sources)
	if err != nil {
		return nil, err
	}
	if tier1Violation != nil {
		return []Violation{*tier1Violation}, nil
	}

	violations := gapFillDirectivesPerLocation(parsed, cfg)
	violations = append(violations, gapFillPossibleTypeExtension(parsed, cfg)...)

	tier3Violations, err := tier3Descriptions(fileSources, parsed, cfg)
	if err != nil {
		return nil, err
	}
	violations = append(violations, tier3Violations...)

	return violations, nil
}

// parsedSchema bundles a validated schema with the raw, pre-merge
// *ast.SchemaDocument the Tier 2 gap-fill passes need for the definition
// and extension names doc.Definitions and doc.Extensions carry --
// gqlparser.LoadSchema only returns the merged, validated *ast.Schema, with
// no way to get at that intermediate document.
type parsedSchema struct {
	schema *ast.Schema
	doc    *ast.SchemaDocument
}

// parseAndValidate parses sources -- with the gqlparser prelude
// prepended -- and validates the result. This is exactly what
// gqlparser.LoadSchema does internally (as of gqlparser/v2@v2.5.36;
// re-verify this still holds on any future gqlparser upgrade), reproduced
// here only to keep the intermediate *ast.SchemaDocument parsedSchema
// needs.
//
// A non-nil Violation return means sources failed Tier 1 validation; the
// *parsedSchema is nil in that case. A non-nil error return means
// parsing or validation failed in some way that isn't itself a Tier 1
// rule violation (a bug, not a lint finding).
func parseAndValidate(sources []*ast.Source) (*parsedSchema, *Violation, error) {
	allSources := make([]*ast.Source, 0, len(sources)+1)
	allSources = append(allSources, validator.Prelude)
	allSources = append(allSources, sources...)

	doc, err := parser.ParseSchemas(allSources...)
	if err != nil {
		v, wrapErr := classifyOrWrap(err, "parse graphql schema")
		return nil, v, wrapErr
	}

	schema, err := validator.ValidateSchemaDocument(doc)
	if err != nil {
		v, wrapErr := classifyOrWrap(err, "validate graphql schema")
		return nil, v, wrapErr
	}

	return &parsedSchema{schema: schema, doc: doc}, nil, nil
}

// classifyOrWrap turns err into a Tier 1 Violation if it is a
// *gqlerror.Error, tagging it via ruleForMessage; any other error is
// wrapped with action for context instead, since it isn't a lint
// finding.
func classifyOrWrap(err error, action string) (*Violation, error) {
	var gqlErr *gqlerror.Error
	if !errors.As(err, &gqlErr) {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	return &Violation{err: gqlErr, Rule: ruleForMessage(gqlErr.Message)}, nil
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
	case strings.HasPrefix(msg, "Enum value ") && strings.HasSuffix(msg, "can only be defined once."):
		return config.RuleUniqueEnumValueNames
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
