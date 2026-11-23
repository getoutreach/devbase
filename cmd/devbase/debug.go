// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Debug logging setup for the devbase CLI.

package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

// configureDebugLogging turns on debug logging for log and the global
// logrus logger when DEVBASE_DEBUG is set.
//
// This lives outside devbase.go because stencil regenerates that file's
// import list, and a formatter moves any import added inside its
// "imports" block out of the block, so a restencil drops it again.
func configureDebugLogging(log *logrus.Logger) {
	if os.Getenv("DEVBASE_DEBUG") == "" {
		return
	}
	log.SetLevel(logrus.DebugLevel)
	logrus.SetLevel(logrus.DebugLevel)
}
