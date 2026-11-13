// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The debug log and the secret masking of the release
// command.

// Package rlog masks secrets in the output of the release command, as
// semantic-release does, and writes its debug log. Both travel in the
// context.
package rlog

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/release/internal/js"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the rules of hide-sensitive.js and definitions/constants.js
// in semantic-release 25.
const (
	// Secure replaces each secret.
	Secure = "[secure]"

	// minSecretSize is SECRET_MIN_SIZE: a shorter value is not masked.
	minSecretSize = 5
)

// secretName matches the ASCII-lowercased name of an environment
// variable that holds a secret.
var secretName = regexp.MustCompile(`token|password|credential|secret|private|key|auth|webhook`)

// Masker replaces secrets in text with Secure. A nil Masker masks
// nothing.
type Masker struct {
	r *strings.Replacer
}

// NewMasker returns a Masker for the secrets in env, as hide-sensitive.js
// finds them: the values of the variables whose names have a secret word,
// except GOPRIVATE, with at least 5 characters after trim. Each value is
// also masked in its encodeURI and encodeURIComponent forms, and in the
// form that the credentials of a URL have. As in a JavaScript object, a
// later item of env replaces the value of an earlier one but keeps its
// position. It returns nil when env has no secret.
func NewMasker(env []string) *Masker {
	values := spec.EnvMap(env)
	var pairs []string
	names, forms := map[string]bool{}, map[string]bool{}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		value, ok := values[name]
		// lodash size counts Unicode symbols; a rune count differs only
		// for combining marks.
		if !ok || names[name] || name == "GOPRIVATE" || !secretName.MatchString(js.ASCIILower(name)) ||
			utf8.RuneCountInString(js.Trim(value)) < minSecretSize {
			continue
		}
		names[name] = true
		for _, f := range []string{
			value, js.EncodeURI(value), js.EncodeURIComponent(value), js.PercentEncode(value, js.URIAuthKeep),
		} {
			if !forms[f] {
				forms[f] = true
				pairs = append(pairs, f, Secure)
			}
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	// At each position, a Replacer takes the first form that matches, as
	// the alternation of the JavaScript regular expression does.
	return &Masker{r: strings.NewReplacer(pairs...)}
}

// Mask returns s with each secret replaced by Secure.
func (m *Masker) Mask(s string) string {
	if m == nil {
		return s
	}
	return m.r.Replace(s)
}

// Writer returns a writer that masks what it writes to w. It masks each
// Write call on its own, so a secret that two calls split is not masked.
func (m *Masker) Writer(w io.Writer) io.Writer {
	if m == nil {
		return w
	}
	return &maskWriter{w: w, r: m.r}
}

// maskWriter masks each write to w.
type maskWriter struct {
	w io.Writer
	r *strings.Replacer
}

// Write writes p to w with the secrets masked, and reports len(p) when
// the write succeeds, because the masked text can have another length.
func (mw *maskWriter) Write(p []byte) (int, error) {
	if _, err := mw.r.WriteString(mw.w, string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// These are the context keys of this package.
type (
	// maskerKey is the key of the Masker.
	maskerKey struct{}

	// debugKey is the key of the debug log.
	debugKey struct{}
)

// WithMasker returns a context that carries m, for the debug log and
// the errors in trace spans.
func WithMasker(ctx context.Context, m *Masker) context.Context {
	return context.WithValue(ctx, maskerKey{}, m)
}

// MaskerFrom returns the Masker of ctx, or nil.
func MaskerFrom(ctx context.Context) *Masker {
	m, _ := ctx.Value(maskerKey{}).(*Masker)
	return m
}

// WithDebug returns a context whose Debugf calls write to w. w must be
// safe for concurrent use, because Build runs its targets in parallel.
func WithDebug(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, debugKey{}, w)
}

// DebugEnabled reports whether ctx has a debug log, so that a caller
// can skip work that only the log needs.
func DebugEnabled(ctx context.Context) bool {
	_, ok := ctx.Value(debugKey{}).(io.Writer)
	return ok
}

// Debugf writes a line to the debug log of ctx, if it has one, with the
// secrets of the Masker of ctx masked.
func Debugf(ctx context.Context, format string, args ...any) {
	if w, ok := ctx.Value(debugKey{}).(io.Writer); ok {
		fmt.Fprint(w, MaskerFrom(ctx).Mask(fmt.Sprintf("debug: "+format+"\n", args...)))
	}
}
