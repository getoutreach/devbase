// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads scripts/devbase.yaml, the per-repo configuration
// file consulted by devbase lint graphql.

// Package config loads scripts/devbase.yaml, the per-repo
// configuration file consulted by devbase lint graphql for exclude
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

// validSeverities is the set of severity values accepted in
// scripts/devbase.yaml rule overrides.
var validSeverities = map[Severity]bool{ //nolint:gochecknoglobals // Why: read-only lookup table.
	SeverityOff:   true,
	SeverityWarn:  true,
	SeverityError: true,
}

// ErrInvalidRule is wrapped by errors returned when a rule entry in
// scripts/devbase.yaml is neither the short form (a severity scalar)
// nor the long form (a [severity, options] sequence), or names an
// unknown severity.
var ErrInvalidRule = errors.New("invalid rule config")

// ErrTier1RuleNotConfigurable is wrapped by the error returned when
// scripts/devbase.yaml overrides a Tier 1 rule's severity. Tier 1 rules
// are enforced by gqlparser while parsing SDL, always at "error"
// severity, and cannot be turned off or downgraded.
var ErrTier1RuleNotConfigurable = errors.New("tier 1 rule severity cannot be overridden")

// Names of the 10 Tier 1 rules: spec validations gqlparser performs for
// free while parsing SDL. Named as constants so internal/graphql/lint
// can tag violations with the same identifiers this package validates
// against.
//
// RuleUniqueEnumValueNames belongs here, not among the gap-fill rules
// in internal/graphql/lint, because the gqlparser/v2 version pinned in
// go.mod already rejects a duplicate enum value on its own, the same
// way it already rejects a duplicate field name for
// RuleUniqueFieldDefinitionNames -- no custom code needed.
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

	// RuleUniqueEnumValueNames requires enum values within an enum to
	// have unique names.
	RuleUniqueEnumValueNames = "unique-enum-value-names"
)

// Tier1RuleNames returns the 10 Tier 1 rule names above, in the order
// they are declared.
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
		RuleUniqueEnumValueNames,
	}
}

// Names of the 2 remaining Tier 2 gap-fill rules: gqlparser partially
// covers each, but a custom pass is still needed to fill the gap.
// Unlike the Tier 1 rules, scripts/devbase.yaml may override their
// severity.
const (
	// RuleUniqueDirectiveNamesPerLocation requires a non-repeatable
	// directive to appear at most once per location in SDL.
	RuleUniqueDirectiveNamesPerLocation = "unique-directive-names-per-location"

	// RulePossibleTypeExtension requires a type extension to reference
	// a type that is actually defined somewhere.
	RulePossibleTypeExtension = "possible-type-extension"
)

// Rule is the per-rule override for a single lint rule. It accepts
// two YAML shapes:
//
//	# Short form: severity only.
//	rule-name: warn
//
//	# Long form: severity plus rule-specific options.
//	rule-name:
//	  - error
//	  - someOption: true
type Rule struct {
	// Severity overrides the rule's default severity.
	Severity Severity

	// Options carries rule-specific options from the long form. It is
	// nil when the short form was used.
	Options map[string]any
}

// var _ yaml.Unmarshaler = (*Rule)(nil) documents, at compile time,
// that UnmarshalYAML's signature actually satisfies yaml.Unmarshaler
// -- go.yaml.in/yaml/v3 also accepts an older unmarshal-func signature
// that this type does not implement, and a mismatched signature would
// otherwise fail silently by reflection.
var _ yaml.Unmarshaler = (*Rule)(nil)

// UnmarshalYAML implements yaml.Unmarshaler for Rule, decoding both
// shapes described above.
func (r *Rule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		severity, err := decodeSeverity(value)
		if err != nil {
			return err
		}
		r.Severity, r.Options = severity, nil
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		return r.unmarshalLongForm(value)
	}
	return fmt.Errorf("%w: expected a severity string or a [severity, options] sequence, got %v",
		ErrInvalidRule, value.Kind)
}

// unmarshalLongForm decodes the [severity, options] sequence form of a
// rule override.
func (r *Rule) unmarshalLongForm(value *yaml.Node) error {
	if len(value.Content) == 0 || len(value.Content) > 2 {
		return fmt.Errorf("%w: sequence form must have 1 or 2 elements, got %d",
			ErrInvalidRule, len(value.Content))
	}

	severity, err := decodeSeverity(value.Content[0])
	if err != nil {
		return err
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

// decodeSeverity decodes node as a Severity and rejects any value
// outside {off, warn, error}.
func decodeSeverity(node *yaml.Node) (Severity, error) {
	var severity Severity
	if err := node.Decode(&severity); err != nil {
		return "", fmt.Errorf("decode rule severity: %w", err)
	}
	if !validSeverities[severity] {
		return "", fmt.Errorf("%w: unknown severity %q", ErrInvalidRule, severity)
	}
	return severity, nil
}

// Lint is the graphql.lint section of scripts/devbase.yaml.
type Lint struct {
	// Exclude is a list of glob patterns for files to skip, relative
	// to the directory Load found scripts/devbase.yaml in.
	Exclude []string `yaml:"exclude"`

	// Rules overrides the severity (and, for some rules, options) of
	// individual lint rules, keyed by rule name. Tier 1 rules always
	// stay at "error" (gqlparser enforces them unconditionally, and
	// this map can never target one -- see ErrTier1RuleNotConfigurable).
	// Every other rule absent from this map, or explicitly set to
	// SeverityOff, does not run at all -- matching @graphql-eslint's own
	// behavior, where a rule is inert until a config opts into it.
	Rules map[string]Rule `yaml:"rules"`

	// Federation, if set, is the Apollo Federation subgraph spec
	// version (for example "v2.3") this repo's own schema links
	// against via `extend schema @link(url: "...", import: [...])`.
	// See internal/graphql/lint/federation.go for the supported
	// versions and directives, and how a mismatched or unrecognized
	// import is reported.
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
func (c *Lint) MergeExcludes(extra ...string) []string {
	merged := make([]string, 0, len(c.Exclude)+len(extra))
	merged = append(merged, c.Exclude...)
	merged = append(merged, extra...)
	return merged
}

// Enabled reports whether the rule should run at all. A Tier 2 or Tier
// 3 rule is enabled only once scripts/devbase.yaml gives it a severity
// other than SeverityOff -- it does not run by default, matching
// @graphql-eslint's own behavior of a rule staying inert until a config
// opts into it. c may be nil (no config file found), in which case
// every rule this method is asked about is disabled.
//
// This method is never consulted for Tier 1 rules: gqlparser enforces
// them unconditionally while parsing SDL, so Files runs them
// regardless of any config.
func (c *Lint) Enabled(rule string) bool {
	if c == nil {
		return false
	}
	rc, ok := c.Rules[rule]
	return ok && rc.Severity != SeverityOff
}

// fileConfig mirrors the top-level shape of scripts/devbase.yaml.
type fileConfig struct {
	GraphQL struct {
		Lint Lint `yaml:"lint"`
	} `yaml:"graphql"`
}

// Load discovers and parses scripts/devbase.yaml for the repository
// containing startDir, and returns the directory it was found in.
// Load walks up from startDir, checking each directory for
// scripts/devbase.yaml, until either the file is found or the
// enclosing git repository's top-level directory is reached
// (whichever comes first); the walk never crosses git repository
// boundaries.
//
// Callers should resolve Lint.Exclude patterns relative to the
// returned directory, not the process's working directory -- matching
// how ESLint, Biome, and Oxlint resolve their own ignore patterns
// relative to the config file rather than cwd.
//
// If no config file is found, Load returns the built-in defaults (no
// excludes, no rule overrides, so every Tier 1 rule stays at "error"
// and every other rule stays off) alongside an empty directory.
func Load(startDir string) (*Lint, string, error) {
	dir, err := discover(startDir)
	if err != nil {
		return nil, "", err
	}
	if dir == "" {
		return &Lint{}, "", nil
	}

	path := filepath.Join(dir, scriptsDevbaseYAML)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(b, &fc); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", path, err)
	}

	if err := validateNoTier1Overrides(fc.GraphQL.Lint.Rules); err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}

	return &fc.GraphQL.Lint, dir, nil
}

// validateNoTier1Overrides rejects a rule override targeting one of the
// 9 Tier 1 rules: gqlparser enforces them while parsing SDL, so their
// severity is always "error" and scripts/devbase.yaml cannot change it.
func validateNoTier1Overrides(rules map[string]Rule) error {
	for _, name := range Tier1RuleNames() {
		if _, overridden := rules[name]; overridden {
			return fmt.Errorf("%w: %s", ErrTier1RuleNotConfigurable, name)
		}
	}
	return nil
}

// discover walks up from startDir looking for scripts/devbase.yaml,
// stopping at (and including) the enclosing git repository's
// top-level directory. It returns the directory scripts/devbase.yaml
// was found in, or an empty string with no error if no config file is
// found before that boundary.
func discover(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path for %s: %w", startDir, err)
	}

	for {
		candidate := filepath.Join(dir, scriptsDevbaseYAML)
		if isRegularFile(candidate) {
			return dir, nil
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
