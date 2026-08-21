// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the lint graphql subcommand's per-rule
// severity handling.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/devbase/v2/internal/graphql/lint"
	"gotest.tools/v3/assert"
)

// writeSchema writes sdl to a schema.graphql file under a fresh
// temporary directory and returns its path.
func writeSchema(t *testing.T, sdl string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.graphql")
	assert.NilError(t, os.WriteFile(path, []byte(sdl), 0o600))
	return path
}

// TestReportViolations confirms lintGraphQL's severity handling, per
// DT-5506: a violation resolves to "error" severity -- and so should
// fail the run -- only when its rule is either absent from cfg.Rules
// (every Tier 1 rule, which scripts/devbase.yaml can never override)
// or explicitly configured to SeverityError; a rule configured to
// SeverityWarn is printed but does not by itself fail the run.
func TestReportViolations(t *testing.T) {
	cases := []struct {
		name         string
		sdl          string
		cfg          *config.Lint
		wantRules    []string
		wantHasError bool
	}{
		{
			name: "warn rule does not fail the run",
			sdl: `
				directive @foo on OBJECT
				type Foo @foo @foo {
					a: String
				}
			`,
			cfg: &config.Lint{
				Rules: map[string]config.Rule{
					config.RuleUniqueDirectiveNamesPerLocation: {Severity: config.SeverityWarn},
				},
			},
			wantRules:    []string{config.RuleUniqueDirectiveNamesPerLocation},
			wantHasError: false,
		},
		{
			name: "error rule alongside a warn rule still fails the run",
			sdl: `
				directive @foo on OBJECT
				type Foo @foo @foo {
					a: String
				}
				extend type Bar {
					b: String
				}
			`,
			cfg: &config.Lint{
				Rules: map[string]config.Rule{
					config.RuleUniqueDirectiveNamesPerLocation: {Severity: config.SeverityWarn},
					config.RulePossibleTypeExtension:           {Severity: config.SeverityError},
				},
			},
			wantRules:    []string{config.RuleUniqueDirectiveNamesPerLocation, config.RulePossibleTypeExtension},
			wantHasError: true,
		},
		{
			name: "tier 1 rule, never in the config map, still fails the run",
			sdl: `
				type Foo { a: String }
				type Foo { b: String }
			`,
			cfg:          nil,
			wantRules:    []string{config.RuleUniqueTypeNames},
			wantHasError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSchema(t, tc.sdl)

			violations, err := lint.Files([]string{path}, tc.cfg)
			assert.NilError(t, err)
			assert.Equal(t, len(violations), len(tc.wantRules))

			var wantOutput bytes.Buffer
			for i, rule := range tc.wantRules {
				assert.Equal(t, violations[i].Rule, rule)
				wantOutput.WriteString(violations[i].String() + "\n")
			}

			var buf bytes.Buffer
			hasError, err := reportViolations(&buf, violations, tc.cfg)
			assert.NilError(t, err)
			assert.Equal(t, hasError, tc.wantHasError)
			assert.Equal(t, buf.String(), wantOutput.String())
		})
	}
}
