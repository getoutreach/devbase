// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Trace spans for the release steps, through the devbase
// CLI tracing.

package release

import (
	"context"
	"errors"

	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/getoutreach/gobox/pkg/trace"
)

// startStep starts the trace span "release.<name>". The caller defers
// the returned function with a pointer to its error result: it records
// the error in the span, with the secrets of the Masker of ctx masked,
// and ends the span. Without a tracer, as in tests, it does nothing.
func startStep(ctx context.Context, name string) (stepCtx context.Context, end func(*error)) {
	ctx = trace.StartSpan(ctx, "release."+name)
	return ctx, func(err *error) {
		if *err != nil {
			//nolint:errcheck,err113 // Why: a masked copy of the error for the span.
			trace.Error(ctx, errors.New(rlog.MaskerFrom(ctx).Mask((*err).Error())))
		}
		trace.End(ctx)
	}
}

// traceStep runs fn in the trace span "release.<name>".
func traceStep(ctx context.Context, name string, fn func(context.Context) error) (err error) {
	ctx, end := startStep(ctx, name)
	defer end(&err)
	return fn(ctx)
}
