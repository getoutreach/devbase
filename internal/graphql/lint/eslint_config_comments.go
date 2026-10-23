// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Honors "# eslint <rule>: <config>" inline configuration
// comments.

// eslint_config_comments.go lets a schema file reconfigure a Tier 2 or
// Tier 3 rule for itself with the comment @graphql-eslint honors:
//
//	# eslint @graphql-eslint/naming-convention: ["error", { FieldDefinition: "camelCase" }]
//	# eslint @graphql-eslint/naming-convention: "off", @graphql-eslint/require-description: "error"
//
// As in ESLint, such a comment applies to its whole file wherever it
// appears, replaces the rule's configured options entirely instead of
// merging with them (so a severity alone leaves the rule with no
// options), and the last comment for a rule wins. A rule is named with
// or without the "@graphql-eslint/" prefix. The value is a severity
// ("off", "warn", "error", 0, 1 or 2) or a [severity, options] list,
// written as YAML flow syntax so unquoted keys work as they do in ESLint.
// A "-- reason" suffix is ignored.
//
// Only the rule's on/off state and options can differ per file: a file's
// "warn" or "error" still reports at the severity scripts/devbase.yaml
// gives the rule, or at "error" for a rule that file leaves unconfigured
// and a comment enables. Tier 1 rules cannot be configured, and an entry
// that names no configurable rule or has an invalid value is ignored.
//
// Rules take a single configuration for the whole schema, so each
// distinct override reruns only its rule over the whole schema and keeps
// the results for the files that declared it. A schema where every file
// carries a different override therefore pays one rerun per file.

package lint

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/lexer"
	"go.yaml.in/yaml/v3"
)

// configurableRules are the rules an inline configuration comment may
// change: every Tier 2 and Tier 3 rule.
//
//nolint:gochecknoglobals // Why: built once, never mutated.
var configurableRules = set.Of(
	config.RuleUniqueDirectiveNamesPerLocation, config.RulePossibleTypeExtension,
	config.RuleRequireDescription, config.RuleDescriptionStyle, config.RuleNoHashtagDescription,
	config.RuleRequireDeprecationReason, config.RuleRequireDeprecationDate, config.RuleNoTypenamePrefix,
	config.RuleNamingConvention, config.RuleNoCaseInsensitiveEnumValuesDuplicates,
	config.RuleAlphabetize, config.RuleNoUnreachableTypes,
)

// eslintSeverities maps ESLint's numeric severities to devbase's names.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated.
var eslintSeverities = map[string]string{"0": "off", "1": "warn", "2": "error"}

// splitTopLevel splits s at each comma that is neither inside brackets
// or braces nor inside a quoted string.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	var quote rune
	escaped := false
	for i, r := range s {
		switch {
		case escaped:
			escaped = false
		case quote != 0:
			escaped = r == '\\'
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '[' || r == '{':
			depth++
		case r == ']' || r == '}':
			depth--
		case r == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// parseEslintRuleConfig parses the value of one "rule: value" entry,
// reporting false if it is not a valid severity or [severity, options].
func parseEslintRuleConfig(value string) (config.Rule, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(value), &doc); err != nil || len(doc.Content) == 0 {
		return config.Rule{}, false
	}

	root := doc.Content[0]
	severity := root
	if root.Kind == yaml.SequenceNode && len(root.Content) > 0 {
		severity = root.Content[0]
	}
	if name, ok := eslintSeverities[severity.Value]; ok && severity.Kind == yaml.ScalarNode {
		severity.Value, severity.Tag = name, "!!str"
	}

	var rule config.Rule
	if err := root.Decode(&rule); err != nil {
		return config.Rule{}, false
	}
	return rule, true
}

// parseEslintConfigComment parses the text of a "#" comment, without the
// leading "#", as an inline configuration comment, returning the rules
// it configures. It returns nil for any other comment.
func parseEslintConfigComment(text string) map[string]config.Rule {
	rest, ok := strings.CutPrefix(strings.TrimSpace(text), "eslint")
	if first, _ := utf8.DecodeRuneInString(rest); !ok || !unicode.IsSpace(first) {
		return nil
	}
	if loc := eslintReasonSeparator.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}

	var rules map[string]config.Rule
	for _, entry := range splitTopLevel(rest) {
		name, value, found := strings.Cut(entry, ":")
		name = strings.TrimPrefix(strings.TrimSpace(name), eslintRulePrefix)
		if !found || !configurableRules.Contains(name) {
			continue
		}
		if rule, ok := parseEslintRuleConfig(value); ok {
			if rules == nil {
				rules = make(map[string]config.Rule)
			}
			rules[name] = rule
		}
	}
	return rules
}

// eslintConfigComments lexes src for inline configuration comments and
// returns the rules they configure, the last comment for a rule winning.
func eslintConfigComments(src *ast.Source) (map[string]config.Rule, error) {
	lx := lexer.New(src)

	overrides := make(map[string]config.Rule)
	for {
		tok, err := lx.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("lex %s: %w", src.Name, err)
		}
		if tok.Kind == lexer.EOF {
			return overrides, nil
		}
		if tok.Kind == lexer.Comment {
			maps.Copy(overrides, parseEslintConfigComment(strings.TrimPrefix(tok.Value, "#")))
		}
	}
}

// ruleOverride is one rule configuration declared by a set of files.
type ruleOverride struct {
	name  string
	rule  config.Rule
	files set.Set[string]
}

// withConfigComments returns violations adjusted for the inline
// configuration comments in fileSources: a file's violations of a rule it
// reconfigures are replaced with the rule's results under that
// configuration. violations must be the result of running runRules on
// fileSources and parsed.
func withConfigComments(fileSources []*ast.Source, parsed *parsedSchema, violations []Violation) ([]Violation, error) {
	overridden := make(map[string]map[string]config.Rule)
	for _, src := range fileSources {
		// Skip lexing files that do not contain "eslint": they cannot hold a
		// configuration comment.
		if !strings.Contains(src.Input, "eslint") {
			continue
		}
		overrides, err := eslintConfigComments(src)
		if err != nil {
			return nil, err
		}
		if len(overrides) > 0 {
			overridden[src.Name] = overrides
		}
	}
	if len(overridden) == 0 {
		return violations, nil
	}

	var kept []Violation
	for _, v := range violations {
		if _, ok := overridden[v.File()][v.Rule]; !ok {
			kept = append(kept, v)
		}
	}

	groups := make(map[string]*ruleOverride)
	for file, overrides := range overridden {
		for name, rule := range overrides {
			// %#v keeps option values of different types, such as 1 and "1",
			// in separate groups.
			key := fmt.Sprintf("%s\x00%s\x00%#v", name, rule.Severity, rule.Options)
			if groups[key] == nil {
				groups[key] = &ruleOverride{name: name, rule: rule, files: make(set.Set[string])}
			}
			groups[key].files.Insert(file)
		}
	}

	for _, key := range slices.Sorted(maps.Keys(groups)) {
		g := groups[key]
		if g.rule.Severity == config.SeverityOff {
			continue
		}
		results, err := runRules(fileSources, parsed, &config.Lint{Rules: map[string]config.Rule{g.name: g.rule}})
		if err != nil {
			return nil, err
		}
		for _, v := range results {
			if v.Rule == g.name && g.files.Contains(v.File()) {
				kept = append(kept, v)
			}
		}
	}
	return kept, nil
}
