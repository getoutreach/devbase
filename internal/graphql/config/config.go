// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads scripts/devbase.yaml, the per-repo configuration
// file consulted by devbase graphql lint.

// Package config loads scripts/devbase.yaml, the per-repo
// configuration file consulted by devbase graphql lint for exclude
// patterns and per-rule severity/option overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// scriptsDevbaseYAML is the config file path, relative to a
// repository's top-level directory.
const scriptsDevbaseYAML = "scripts/devbase.yaml"

// Severity is the severity level of a lint rule.
type Severity string

const (
	// SeverityOff disables a rule.
	SeverityOff Severity = "off"

	// SeverityWarn reports a rule's violations without failing the lint run.
	SeverityWarn Severity = "warn"

	// SeverityError reports a rule's violations and fails the lint run.
	SeverityError Severity = "error"
)

// ErrInvalidRuleConfig is wrapped by errors returned when a rule entry
// in scripts/devbase.yaml is neither the short form (a severity
// scalar) nor the long form (a [severity, options] sequence).
var ErrInvalidRuleConfig = errors.New("invalid rule config")

// ErrTier1RuleNotConfigurable is wrapped by the error returned when
// scripts/devbase.yaml overrides a Tier 1 rule's severity. Tier 1 rules
// are enforced by gqlparser while parsing SDL, always at "error"
// severity, and cannot be turned off or downgraded.
var ErrTier1RuleNotConfigurable = errors.New("tier 1 rule severity cannot be overridden")

// Names of the 9 Tier 1 rules: spec validations gqlparser performs for
// free while parsing SDL. Named as constants so internal/graphql/lint
// can tag violations with the same identifiers this package validates
// against.
const (
	// RuleUniqueDirectiveNames requires directive definitions to have
	// unique names.
	RuleUniqueDirectiveNames = "unique-directive-names"

	// RuleUniqueFieldDefinitionNames requires fields within a type to
	// have unique names.
	RuleUniqueFieldDefinitionNames = "unique-field-definition-names"

	// RuleUniqueOperationTypes requires at most one query, mutation,
	// and subscription root type.
	RuleUniqueOperationTypes = "unique-operation-types"

	// RuleUniqueTypeNames requires type definitions to have unique
	// names.
	RuleUniqueTypeNames = "unique-type-names"

	// RuleKnownArgumentNames requires arguments to be defined in the
	// schema.
	RuleKnownArgumentNames = "known-argument-names"

	// RuleKnownDirectives requires directives to be defined and used
	// in a valid location.
	RuleKnownDirectives = "known-directives"

	// RuleKnownTypeNames requires referenced types to exist in the
	// schema.
	RuleKnownTypeNames = "known-type-names"

	// RuleProvidedRequiredArguments requires required arguments to be
	// provided.
	RuleProvidedRequiredArguments = "provided-required-arguments"

	// RuleLoneSchemaDefinition allows at most one schema definition.
	RuleLoneSchemaDefinition = "lone-schema-definition"
)

// Tier1RuleNames returns the 9 Tier 1 rule names above, in the order RFC
// 0006 presents them.
func Tier1RuleNames() []string {
	return []string{
		RuleUniqueDirectiveNames,
		RuleUniqueFieldDefinitionNames,
		RuleUniqueOperationTypes,
		RuleUniqueTypeNames,
		RuleKnownArgumentNames,
		RuleKnownDirectives,
		RuleKnownTypeNames,
		RuleProvidedRequiredArguments,
		RuleLoneSchemaDefinition,
	}
}

// RuleConfig is the per-rule override for a single lint rule. It
// accepts two YAML shapes:
//
//	# Short form: severity only.
//	rule-name: warn
//
//	# Long form: severity plus rule-specific options.
//	rule-name:
//	  - error
//	  - someOption: true
type RuleConfig struct {
	// Severity overrides the rule's default severity.
	Severity Severity

	// Options carries rule-specific options from the long form. It is
	// nil when the short form was used.
	Options map[string]any
}

// UnmarshalYAML implements yaml.Unmarshaler for RuleConfig, decoding
// both shapes described above.
func (r *RuleConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var severity Severity
		if err := value.Decode(&severity); err != nil {
			return fmt.Errorf("decode rule severity: %w", err)
		}
		r.Severity, r.Options = severity, nil
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		return r.unmarshalLongForm(value)
	}
	return fmt.Errorf("%w: expected a severity string or a [severity, options] sequence, got %v",
		ErrInvalidRuleConfig, value.Kind)
}

// unmarshalLongForm decodes the [severity, options] sequence form of a
// rule override.
func (r *RuleConfig) unmarshalLongForm(value *yaml.Node) error {
	if len(value.Content) == 0 || len(value.Content) > 2 {
		return fmt.Errorf("%w: sequence form must have 1 or 2 elements, got %d",
			ErrInvalidRuleConfig, len(value.Content))
	}

	var severity Severity
	if err := value.Content[0].Decode(&severity); err != nil {
		return fmt.Errorf("decode rule severity: %w", err)
	}

	var options map[string]any
	if len(value.Content) == 2 {
		if err := value.Content[1].Decode(&options); err != nil {
			return fmt.Errorf("decode rule options: %w", err)
		}
	}

	r.Severity, r.Options = severity, options
	return nil
}

// LintConfig is the graphql.lint section of scripts/devbase.yaml.
type LintConfig struct {
	// Exclude is a list of glob patterns for files to skip.
	Exclude []string `yaml:"exclude"`

	// Rules overrides the severity (and, for some rules, options) of
	// individual lint rules, keyed by rule name. A rule absent from
	// this map keeps its built-in default severity of "error".
	Rules map[string]RuleConfig `yaml:"rules"`

	// Federation, if set, is the Apollo Federation subgraph spec
	// version (for example "v2.3") this repo's own schema links
	// against via `extend schema @link(url: "https://specs.apollo.dev/
	// federation/v2.3", import: [...])`. gqlparser has no built-in
	// notion of @link or anything it imports -- composition tooling
	// injects those, they're never declared via `directive @...` SDL
	// in a subgraph's own files -- so setting this tells
	// internal/graphql/lint to synthesize definitions for exactly the
	// directives the schema's own @link imports. See that package's
	// federation support for which versions and directives it
	// understands, and how a mismatched or unrecognized import is
	// reported.
	Federation string `yaml:"federation"`

	// Scalars lists custom scalar type names that are registered
	// outside this repo's *.graphql files -- for example, at runtime
	// in application code -- and so are never declared via `scalar X`
	// SDL. Each name is merged into the schema as a bare `scalar X`
	// declaration.
	Scalars []string `yaml:"scalars"`
}

// MergeExcludes returns the config's exclude patterns extended with
// extra patterns, e.g. from repeatable --exclude CLI flags. The
// config file's list is always kept, never replaced.
func (c *LintConfig) MergeExcludes(extra ...string) []string {
	merged := make([]string, 0, len(c.Exclude)+len(extra))
	merged = append(merged, c.Exclude...)
	merged = append(merged, extra...)
	return merged
}

// fileConfig mirrors the top-level shape of scripts/devbase.yaml.
type fileConfig struct {
	GraphQL struct {
		Lint LintConfig `yaml:"lint"`
	} `yaml:"graphql"`
}

// Load discovers and parses scripts/devbase.yaml for the repository
// containing startDir. It walks up from startDir, checking each
// directory for scripts/devbase.yaml, until either the file is found
// or the enclosing git repository's top-level directory is reached
// (whichever comes first); the walk never crosses git repository
// boundaries.
//
// If no config file is found, Load returns the built-in defaults: no
// excludes and no rule overrides, meaning every rule stays at its
// default severity of "error".
func Load(startDir string) (*LintConfig, error) {
	path, err := discover(startDir)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &LintConfig{}, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(b, &fc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if err := validateNoTier1Overrides(fc.GraphQL.Lint.Rules); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &fc.GraphQL.Lint, nil
}

// validateNoTier1Overrides rejects a rule override targeting one of the
// 9 Tier 1 rules: gqlparser enforces them while parsing SDL, so their
// severity is always "error" and scripts/devbase.yaml cannot change it.
func validateNoTier1Overrides(rules map[string]RuleConfig) error {
	for _, name := range Tier1RuleNames() {
		if _, overridden := rules[name]; overridden {
			return fmt.Errorf("%w: %s", ErrTier1RuleNotConfigurable, name)
		}
	}
	return nil
}

// discover walks up from startDir looking for scripts/devbase.yaml,
// stopping at (and including) the enclosing git repository's
// top-level directory. It returns an empty path, with no error, if no
// config file is found before that boundary.
func discover(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path for %s: %w", startDir, err)
	}

	for {
		candidate := filepath.Join(dir, scriptsDevbaseYAML)
		if isRegularFile(candidate) {
			return candidate, nil
		}

		if isGitRoot(dir) {
			return "", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding a git
			// repository; stop the same way as at a git root.
			return "", nil
		}
		dir = parent
	}
}

// isGitRoot reports whether dir is the top level of a git repository,
// i.e. dir/.git exists as either a directory (a normal clone) or a
// file (a worktree or submodule).
func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// isRegularFile reports whether path exists and is a regular file.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
