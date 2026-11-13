// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests that apply to every check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRespectingCanceledContext(t *testing.T) {
	validOwner, err := NewValidOwner(ValidOwnerConfig{Repository: "org/repo"}, nil)
	assert.NilError(t, err)

	checkers := []Checker{
		NewDuplicatedPattern(),
		NewFileExist(),
		NewValidSyntax(),
		NewAvoidShadowing(),
		NewNotOwnedFile(NotOwnedFileConfig{}),
		validOwner,
	}

	for _, checker := range checkers {
		t.Run(checker.Name(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			in := loadInput(t, fixtureValidCodeowners)
			in.RepoDir = newGitRepo(t, "a.go")

			out, err := checker.Check(ctx, in)
			assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
			assert.Equal(t, len(out.Issues), 0)
		})
	}
}
