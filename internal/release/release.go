// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Computes semantic-release versions and notes, and builds
// GoReleaser-compatible archives.

// Package release computes semantic-release versions and release notes,
// and builds GoReleaser-compatible archives, from the release model in
// package spec. LoadRepo maps the release section of scripts/devbase.yaml,
// or the .releaserc.yaml and .goreleaser.yml files, into that model;
// nothing else in the package reads them.
package release
