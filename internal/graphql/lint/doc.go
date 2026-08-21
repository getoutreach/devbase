// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Runs the Tier 1 spec validation rules from RFC 0006
// against GraphQL SDL files and formats the results as lint violations.

// Package lint runs the Tier 1 rule tier from RFC 0006
// (dt-rfcs/rfcs/0006-migrate-graphql-linting-to-go.md) against a
// repository's *.graphql files: the 9 @graphql-eslint rules that
// gqlparser/v2 enforces for free while parsing SDL, needing no custom
// rule code. FindGraphQLFiles discovers the files to lint, respecting
// scripts/devbase.yaml's exclude patterns; LintFiles parses them as one
// combined schema via gqlparser.LoadSchema and turns any resulting parse
// error into a Violation tagged with the Tier 1 rule name it corresponds
// to, using the classification verified in lint_test.go.
//
// gqlparser.LoadSchema stops at the first validation error it finds, so
// LintFiles can only ever report one violation per run; fixing it and
// re-running surfaces the next one, the same behavior a contributor
// would see running gqlparser-based tooling directly.
//
// go.mod pins github.com/vektah/gqlparser/v2 to v2.5.36 (the latest
// v2.5.x release as of 2026-08-21), not the v2.5.16 RFC 0006's "Extra
// spec validations" appendix was audited against; that section has an
// addendum recording the one behavioral difference found while
// re-verifying against v2.5.36, relevant to the future
// unique-enum-value-names (Tier 2) gap-fill work.
package lint
