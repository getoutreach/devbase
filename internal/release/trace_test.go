//go:build or_test

// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the trace spans of the release steps.

package release

import (
	"slices"
	"testing"

	"github.com/getoutreach/gobox/pkg/trace/tracetest"
	"gotest.tools/v3/assert"
)

func TestPublishTraceSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	defer recorder.Close()

	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	assert.NilError(t, p.publish(repo, plan))

	ended := recorder.Ended()
	names := make([]string, 0, len(ended))
	for _, s := range ended {
		names = append(names, s["name"].(string))
	}
	// The plan of planAndBuild and each publish step have a span.
	for _, want := range []string{
		"release.plan", "release.publish.assets", "release.publish.state", "release.publish.generate_notes",
		"release.publish.prepare", "release.publish.tag", "release.publish.publish", "release.publish.success",
	} {
		assert.Assert(t, slices.Contains(names, want), "%s not in %v", want, names)
	}
}
