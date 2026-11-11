// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Splits a GoReleaser before hook into arguments, as
// GoReleaser 1.20 does with github.com/caarlos0/go-shellwords.
//
// splitShellWords is ported from Parser.Parse of
// github.com/caarlos0/go-shellwords v1.0.12, with ParseEnv and
// ParseBacktick off, which is how GoReleaser calls it. That code has this
// license:
//
// The MIT License (MIT)
//
// Copyright (c) 2017 Yasuhiro Matsumoto
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
// IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
// CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
// TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
// SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package release

import (
	"errors"
	"strings"
)

// errInvalidCommandLine is the go-shellwords error for unbalanced quotes,
// a trailing backslash, or a "(" that does not follow "$".
var errInvalidCommandLine = errors.New("invalid command line string")

// shellWordsState is the parser state of splitShellWords.
type shellWordsState struct {
	escaped, doubleQuoted, singleQuoted, backQuote, dollarQuote bool
}

// quoted reports whether the parser is inside any kind of quote.
func (s *shellWordsState) quoted() bool {
	return s.singleQuoted || s.doubleQuoted || s.backQuote || s.dollarQuote
}

// splitShellWords splits a command line into arguments. Like
// go-shellwords, it stops at the first unquoted ";", "&", "|", "<" or
// ">" and ignores the rest of the line, and it keeps backticks and
// "$(...)" as text.
//
//nolint:gocyclo // Why: a direct port of one state machine.
func splitShellWords(line string) ([]string, error) {
	args := []string{}
	buf := ""
	var st shellWordsState
	// got is false before an argument starts. A quoted empty string
	// ("" or '') starts an argument.
	got := false
loop:
	for _, r := range line {
		if st.escaped {
			buf += string(r)
			st.escaped = false
			got = true
			continue
		}
		if r == '\\' {
			if st.singleQuoted {
				buf += string(r)
			} else {
				st.escaped = true
			}
			continue
		}
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			if st.quoted() {
				buf += string(r)
			} else if got {
				args = append(args, buf)
				buf, got = "", false
			}
			continue
		}

		switch r {
		case '`':
			if !st.singleQuoted && !st.doubleQuoted && !st.dollarQuote {
				st.backQuote = !st.backQuote
			}
		case ')':
			if !st.singleQuoted && !st.doubleQuoted && !st.backQuote {
				st.dollarQuote = !st.dollarQuote
			}
		case '(':
			if !st.singleQuoted && !st.doubleQuoted && !st.backQuote {
				if st.dollarQuote || !strings.HasSuffix(buf, "$") {
					return nil, errInvalidCommandLine
				}
				st.dollarQuote = true
				buf += "("
				continue
			}
		case '"':
			if !st.singleQuoted && !st.dollarQuote {
				if st.doubleQuoted {
					got = true
				}
				st.doubleQuoted = !st.doubleQuoted
				continue
			}
		case '\'':
			if !st.doubleQuoted && !st.dollarQuote {
				if st.singleQuoted {
					got = true
				}
				st.singleQuoted = !st.singleQuoted
				continue
			}
		case ';', '&', '|', '<', '>':
			if !st.quoted() {
				// A redirect such as "2>" drops its file descriptor.
				if r == '>' && buf != "" && buf[0] >= '0' && buf[0] <= '9' {
					got = false
				}
				break loop
			}
		}

		got = true
		buf += string(r)
	}

	if got {
		args = append(args, buf)
	}
	if st.escaped || st.quoted() {
		return nil, errInvalidCommandLine
	}
	return args, nil
}
