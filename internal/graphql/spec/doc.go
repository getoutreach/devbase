// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Documents and empirically verifies gqlparser/v2's coverage
// of the Tier 1 spec validation rules from RFC 0006.

// Package spec empirically verifies the Tier 1 rule tier from RFC 0006
// (dt-rfcs/rfcs/0006-migrate-graphql-linting-to-go.md): the 9
// @graphql-eslint rules that gqlparser/v2 enforces for free while parsing
// SDL via gqlparser.LoadSchema, needing no custom lint code. See
// gqlparser_coverage_test.go for the per-rule fixtures and the
// unique-operation-types gap they document.
//
// go.mod pins github.com/vektah/gqlparser/v2 to v2.5.36 (the latest v2.5.x
// release as of 2026-08-21), not the v2.5.16 the RFC's appendix was
// audited against. All 9 rules hold under v2.5.36; the "Extra spec
// validations performed by gqlparser/v2" section of RFC 0006 has an
// addendum recording one behavioral difference found while re-verifying
// against v2.5.36, relevant to the future unique-enum-value-names (Tier 2)
// gap-fill work.
package spec
