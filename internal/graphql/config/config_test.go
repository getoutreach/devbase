// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the scripts/devbase.yaml config loader.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// initGitDir marks dir as a git repository root by creating a .git
// directory in it, mimicking a real clone closely enough for
// isGitRoot's os.Stat check.
func initGitDir(t *testing.T, dir string) {
	t.Helper()
	assert.NilError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
}

// writeConfig writes contents to scripts/devbase.yaml under repoRoot,
// creating the scripts directory if needed.
func writeConfig(t *testing.T, repoRoot, contents string) {
	t.Helper()
	scriptsDir := filepath.Join(repoRoot, "scripts")
	assert.NilError(t, os.MkdirAll(scriptsDir, 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(scriptsDir, "devbase.yaml"), []byte(contents), 0o600))
}

func TestLoadReturnsDefaultsWhenNoConfigFileExists(t *testing.T) {
	repoRoot := t.TempDir()
	initGitDir(t, repoRoot)

	nested := filepath.Join(repoRoot, "internal", "graphql", "schema")
	assert.NilError(t, os.MkdirAll(nested, 0o755))

	got, configDir, err := Load(nested)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, &Lint{})
	assert.Equal(t, configDir, "")
}

func TestLoadWalksUpUntilConfigFileFound(t *testing.T) {
	repoRoot := t.TempDir()
	initGitDir(t, repoRoot)
	writeConfig(t, repoRoot, `
graphql:
  lint:
    exclude:
      - "**/shared.graphql"
`)

	nested := filepath.Join(repoRoot, "internal", "graphql", "schema")
	assert.NilError(t, os.MkdirAll(nested, 0o755))

	got, configDir, err := Load(nested)
	assert.NilError(t, err)
	assert.DeepEqual(t, got.Exclude, []string{"**/shared.graphql"})
	// The returned directory is where scripts/devbase.yaml was found,
	// not startDir, so callers can anchor Exclude patterns to it
	// instead of the process's working directory.
	assert.Equal(t, configDir, repoRoot)
}

func TestLoadStopsAtGitRootWithoutCrossingIntoParentRepo(t *testing.T) {
	outerRoot := t.TempDir()
	initGitDir(t, outerRoot)
	// A config file above the inner repo's root must never be picked
	// up -- the walk must not cross the inner repo's boundary.
	writeConfig(t, outerRoot, `
graphql:
  lint:
    exclude:
      - "**/should-not-be-seen.graphql"
`)

	innerRoot := filepath.Join(outerRoot, "vendor", "inner-repo")
	assert.NilError(t, os.MkdirAll(innerRoot, 0o755))
	initGitDir(t, innerRoot)

	nested := filepath.Join(innerRoot, "internal", "graphql")
	assert.NilError(t, os.MkdirAll(nested, 0o755))

	got, configDir, err := Load(nested)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, &Lint{})
	assert.Equal(t, configDir, "")
}

func TestLoadParsesRuleOverrides(t *testing.T) {
	cases := []struct {
		name      string
		rulesYAML string
		wantRules map[string]Rule
		wantErrIs error
	}{
		{
			name: "short form",
			rulesYAML: `
      require-deprecation-date: warn
      alphabetize: off
`,
			wantRules: map[string]Rule{
				"require-deprecation-date": {Severity: SeverityWarn},
				"alphabetize":              {Severity: SeverityOff},
			},
		},
		{
			name: "long form with options",
			rulesYAML: `
      naming-convention:
        - error
        - types: PascalCase
          FieldDefinition: camelCase
`,
			wantRules: map[string]Rule{
				"naming-convention": {
					Severity: SeverityError,
					Options: map[string]any{
						"types":           "PascalCase",
						"FieldDefinition": "camelCase",
					},
				},
			},
		},
		{
			name: "long form without options",
			rulesYAML: `
      alphabetize:
        - warn
`,
			wantRules: map[string]Rule{
				"alphabetize": {Severity: SeverityWarn},
			},
		},
		{
			name: "invalid shape",
			rulesYAML: `
      alphabetize:
        nested: mapping
`,
			wantErrIs: ErrInvalidRule,
		},
		{
			name: "invalid sequence length",
			rulesYAML: `
      alphabetize:
        - warn
        - {}
        - extra
`,
			wantErrIs: ErrInvalidRule,
		},
		{
			name: "unknown short-form severity",
			rulesYAML: `
      alphabetize: eror
`,
			wantErrIs: ErrInvalidRule,
		},
		{
			name: "unknown long-form severity",
			rulesYAML: `
      alphabetize:
        - false
`,
			wantErrIs: ErrInvalidRule,
		},
		{
			name: "severity is case-sensitive",
			rulesYAML: `
      alphabetize: Error
`,
			wantErrIs: ErrInvalidRule,
		},
		{
			name: "tier 1 rule override rejected",
			rulesYAML: `
      unique-type-names: warn
`,
			wantErrIs: ErrTier1RuleNotConfigurable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			initGitDir(t, repoRoot)
			writeConfig(t, repoRoot, "graphql:\n  lint:\n    rules:\n"+tc.rulesYAML)

			got, _, err := Load(repoRoot)
			if tc.wantErrIs != nil {
				assert.ErrorIs(t, err, tc.wantErrIs)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got.Rules, tc.wantRules)
		})
	}
}

func TestLoadParsesFederationAndScalars(t *testing.T) {
	repoRoot := t.TempDir()
	initGitDir(t, repoRoot)
	writeConfig(t, repoRoot, `
graphql:
  lint:
    federation: v2.3
    scalars:
      - Datetime
      - JSON
`)

	got, _, err := Load(repoRoot)
	assert.NilError(t, err)
	assert.Equal(t, got.Federation, "v2.3")
	assert.DeepEqual(t, got.Scalars, []string{"Datetime", "JSON"})
}

func TestMergeExcludesIsAdditiveAndDoesNotMutateConfig(t *testing.T) {
	c := &Lint{Exclude: []string{"**/shared.graphql"}}

	got := c.MergeExcludes("**/generated/*.graphql", "**/vendor/**")

	assert.DeepEqual(t, got, []string{"**/shared.graphql", "**/generated/*.graphql", "**/vendor/**"})
	assert.DeepEqual(t, c.Exclude, []string{"**/shared.graphql"})
}

func TestMergeExcludesWithNoExtraReturnsConfigExcludes(t *testing.T) {
	c := &Lint{Exclude: []string{"**/shared.graphql"}}
	assert.DeepEqual(t, c.MergeExcludes(), []string{"**/shared.graphql"})
}

func TestEnabledIsFalseByDefault(t *testing.T) {
	var nilConfig *Lint
	assert.Equal(t, nilConfig.Enabled("possible-type-extension"), false)

	c := &Lint{}
	assert.Equal(t, c.Enabled("possible-type-extension"), false)
}

func TestEnabledRequiresAnExplicitNonOffSeverity(t *testing.T) {
	cases := []struct {
		name     string
		severity Severity
		want     bool
	}{
		{name: "off", severity: SeverityOff, want: false},
		{name: "warn", severity: SeverityWarn, want: true},
		{name: "error", severity: SeverityError, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Lint{Rules: map[string]Rule{"possible-type-extension": {Severity: tc.severity}}}
			assert.Equal(t, c.Enabled("possible-type-extension"), tc.want)
		})
	}
}

func TestSeverityOfDefaultsToErrorWhenRuleIsNotInTheMap(t *testing.T) {
	var nilConfig *Lint
	assert.Equal(t, nilConfig.SeverityOf("unique-type-names"), SeverityError)

	c := &Lint{}
	assert.Equal(t, c.SeverityOf("unique-type-names"), SeverityError)
}

func TestSeverityOfUsesTheConfiguredSeverityWhenRuleIsInTheMap(t *testing.T) {
	cases := []struct {
		name     string
		severity Severity
	}{
		{name: "warn", severity: SeverityWarn},
		{name: "error", severity: SeverityError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Lint{Rules: map[string]Rule{"possible-type-extension": {Severity: tc.severity}}}
			assert.Equal(t, c.SeverityOf("possible-type-extension"), tc.severity)
		})
	}
}
