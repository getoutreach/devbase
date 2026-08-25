// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 require-deprecation-reason and
// require-deprecation-date rules.

// deprecation.go implements 2 (of 10) Tier 3 custom rules:
// require-deprecation-reason (every applied @deprecated usage must
// have a non-empty "reason" argument) and require-deprecation-date
// (every applied @deprecated usage must have a valid, not-yet-passed
// deletion date).
//
// Both read a descriptionSite's applied "deprecated" directive usage
// (descriptionSite.directives, populated by descriptions.go's
// groupDescriptionSites) rather than the schema's resolved "deprecated"
// DirectiveDefinition. This is required, not just a style choice:
// gqlparser pre-loads the built-in @deprecated directive (reason only)
// into every schema, and if a repository's own SDL re-declares
// @deprecated with a different signature (for example, adding a
// deletionDate argument), gqlparser does not error. It just keeps
// overwriting schema.Directives["deprecated"] as it processes each
// declaration in turn, so the definition that ends up canonical
// depends on declaration order rather than being reliably either one.
// Reading each usage site's own applied arguments directly sidesteps
// that entirely, matching @graphql-eslint's own approach.
//
// A schema can only reach either rule with a "deprecated" usage at a
// location the definition that ends up canonical actually allows:
// gqlparser's known-directives validation would otherwise have already
// rejected it as a Tier 1 violation before Files ever runs Tier 3.
// Neither rule needs to check locations itself.

package lint

import (
	"regexp"
	"strings"
	"time"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// requireDeprecationReasonViolations reports a RuleRequireDeprecationReason
// violation for every applied "deprecated" usage among sites whose "reason"
// argument is missing, or blank once its literal value is trimmed,
// matching @graphql-eslint's own String(value).trim() check. That check is
// why a non-string literal reason (for example `reason: 0`) still counts
// as present.
func requireDeprecationReasonViolations(sites []descriptionSite) []Violation {
	violations := make([]Violation, 0, len(sites))
	for i := range sites {
		s := &sites[i]
		dir := s.directives.ForName("deprecated")
		if dir == nil {
			continue
		}

		reason := dir.Arguments.ForName("reason")
		if reason != nil && strings.TrimSpace(reason.Value.Raw) != "" {
			continue
		}

		violations = append(violations, Violation{
			err:  gqlerror.ErrorPosf(dir.Position, "Deprecation reason is required for %s.", s.nodeName()),
			Rule: config.RuleRequireDeprecationReason,
		})
	}
	return violations
}

// deletionDateLayout is the "DD/MM/YYYY" time.Parse layout
// require-deprecation-date's deletion-date argument must use, matching
// @graphql-eslint's own format.
const deletionDateLayout = "02/01/2006"

// deletionDateShape matches deletionDateLayout's digit shape, independent
// of whether the value is a real calendar date. requireDeprecationDateViolations
// checks the two separately, the same way @graphql-eslint's own DATE_REGEX
// and Date.parse do.
var deletionDateShape = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)

// requireDeprecationDateOptions is require-deprecation-date's
// scripts/devbase.yaml options, decoded from RuleConfig.Options.
type requireDeprecationDateOptions struct {
	// argumentName is the @deprecated argument require-deprecation-date
	// checks, matching @graphql-eslint's own "argumentName" option.
	argumentName string
}

// parseRequireDeprecationDateOptions decodes opts (RuleConfig.Options)
// into requireDeprecationDateOptions, defaulting argumentName to
// "deletionDate" if opts is nil or its "argumentName" key isn't a
// non-empty string.
func parseRequireDeprecationDateOptions(opts map[string]any) requireDeprecationDateOptions {
	name, _ := opts["argumentName"].(string)
	if name == "" {
		name = "deletionDate"
	}
	return requireDeprecationDateOptions{argumentName: name}
}

// requireDeprecationDateViolations reports a RuleRequireDeprecationDate
// violation for every applied "deprecated" usage among sites that is
// missing opts.argumentName, has it in the wrong "DD/MM/YYYY" shape, or
// names a calendar date that does not exist. A usage whose deletion date
// has already passed as of now is instead reported as removable, since
// there is nothing left to require.
func requireDeprecationDateViolations(sites []descriptionSite, opts requireDeprecationDateOptions, now time.Time) []Violation {
	violations := make([]Violation, 0, len(sites))
	for i := range sites {
		s := &sites[i]
		dir := s.directives.ForName("deprecated")
		if dir == nil {
			continue
		}

		arg := dir.Arguments.ForName(opts.argumentName)
		if arg == nil {
			violations = append(violations, Violation{
				err: gqlerror.ErrorPosf(dir.Position,
					`Directive "@deprecated" must have a deletion date for %s`, s.nodeName()),
				Rule: config.RuleRequireDeprecationDate,
			})
			continue
		}

		raw := arg.Value.Raw
		if !deletionDateShape.MatchString(raw) {
			violations = append(violations, Violation{
				err: gqlerror.ErrorPosf(arg.Value.Position,
					`Deletion date must be in format "DD/MM/YYYY" for %s`, s.nodeName()),
				Rule: config.RuleRequireDeprecationDate,
			})
			continue
		}

		date, err := time.Parse(deletionDateLayout, raw)
		if err != nil {
			violations = append(violations, Violation{
				err:  gqlerror.ErrorPosf(arg.Value.Position, `Invalid %q deletion date for %s`, raw, s.nodeName()),
				Rule: config.RuleRequireDeprecationDate,
			})
			continue
		}

		if now.After(date) {
			violations = append(violations, Violation{
				err:  gqlerror.ErrorPosf(s.pos, "%s can be removed", s.nodeName()),
				Rule: config.RuleRequireDeprecationDate,
			})
		}
	}
	return violations
}

// tier3Deprecation runs require-deprecation-reason and
// require-deprecation-date against every applied "deprecated" usage
// among sitesByFile's sites. Neither runs unless cfg enables it
// (config.Lint.Enabled).
func tier3Deprecation(sitesByFile map[*ast.Source][]descriptionSite, cfg *config.Lint) []Violation {
	reasonEnabled := cfg.Enabled(config.RuleRequireDeprecationReason)
	dateEnabled := cfg.Enabled(config.RuleRequireDeprecationDate)
	if !reasonEnabled && !dateEnabled {
		return nil
	}

	dateOpts := parseRequireDeprecationDateOptions(cfg.Options(config.RuleRequireDeprecationDate))
	now := time.Now()

	var violations []Violation
	for _, sites := range sitesByFile {
		if reasonEnabled {
			violations = append(violations, requireDeprecationReasonViolations(sites)...)
		}
		if dateEnabled {
			violations = append(violations, requireDeprecationDateViolations(sites, dateOpts, now)...)
		}
	}
	return violations
}
