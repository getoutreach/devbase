// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the DEVBASE_DEBUG logging setup.

package main

import (
	"testing"

	"github.com/sirupsen/logrus"
	"gotest.tools/v3/assert"
)

func TestConfigureDebugLogging(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		wantLevel logrus.Level
	}{
		{name: "unset leaves the level alone", env: "", wantLevel: logrus.InfoLevel},
		{name: "set enables debug logging", env: "1", wantLevel: logrus.DebugLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEVBASE_DEBUG", tt.env)
			// configureDebugLogging also changes the global logger, so
			// restore it when the test ends.
			origLevel := logrus.GetLevel()
			t.Cleanup(func() { logrus.SetLevel(origLevel) })
			logrus.SetLevel(logrus.InfoLevel)

			log := logrus.New()
			configureDebugLogging(log)

			assert.Equal(t, log.GetLevel(), tt.wantLevel)
			assert.Equal(t, logrus.GetLevel(), tt.wantLevel)
		})
	}
}
