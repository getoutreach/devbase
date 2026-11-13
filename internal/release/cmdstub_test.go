// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Fakes of the registry commands for the tests, with the
// gobox execstub package.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/gobox/pkg/exec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// anyArg in an expected command matches any one argument, such as the
// path of a temporary npm config.
const anyArg = "<any>"

// errExit stands for the *exec.ExitError of a failed command in the
// test tables. See assertErrIs.
var errExit = errors.New("exit status 1")

// assertErrIs checks that err wraps want, or an *exec.ExitError when
// want is errExit.
func assertErrIs(t *testing.T, err, want error) {
	t.Helper()
	if errors.Is(want, errExit) {
		var exitErr *exec.ExitError
		assert.Assert(t, errors.As(err, &exitErr), "want an exit error, got %v", err)
		return
	}
	assert.ErrorIs(t, err, want)
}

// stubCmd is an expected command and its result.
type stubCmd struct {
	argv []string
	res  execstub.Result
}

// cmdStubs fakes the registry commands. Each expected command answers
// one command, in the order of expect. A command that is not expected,
// a command out of order, and an expected command that does not run
// fail the test.
type cmdStubs struct {
	t     *testing.T
	stubs *execstub.Stubs

	// spy records the commands in the order that they were created.
	spy *execstub.SpyExec

	// want are the expected commands, in order.
	want []execstub.Matcher
}

// newCmdStubs returns a cmdStubs that checks the order of the commands
// at the end of the test.
func newCmdStubs(t *testing.T) *cmdStubs {
	t.Helper()
	stubs := execstub.NewStubs(t)
	s := &cmdStubs{t: t, stubs: stubs, spy: execstub.NewSpyExec(stubs)}
	t.Cleanup(s.checkOrder)
	return s
}

// expect adds the command argv, which gives res.
func (s *cmdStubs) expect(res execstub.Result, argv ...string) {
	m := execstub.MatchFunc(strings.Join(argv, " "), func(name string, args []string) bool {
		return slices.EqualFunc(argv, append([]string{name}, args...), func(want, got string) bool {
			return want == anyArg || want == got
		})
	})
	s.want = append(s.want, m)
	s.stubs.Register(m, res)
}

// expectAll adds each command of cmds, in order.
func (s *cmdStubs) expectAll(cmds []stubCmd) {
	for _, c := range cmds {
		s.expect(c.res, c.argv...)
	}
}

// calls returns the commands that ran.
func (s *cmdStubs) calls() []execstub.Call {
	return s.spy.Calls
}

// checkOrder fails the test for each command that ran in the place of
// another expected command. execstub reports the commands that are not
// expected, and the expected commands that did not run.
func (s *cmdStubs) checkOrder() {
	for i, c := range s.calls() {
		if i < len(s.want) && !s.want[i].Match(c.Name, c.Args) {
			s.t.Errorf("command %d is %s, want %s", i, strings.Join(append([]string{c.Name}, c.Args...), " "), s.want[i])
		}
	}
}

// cmdOK is the result of a command that succeeds with stdout.
func cmdOK(stdout string) execstub.Result {
	return execstub.Result{Stdout: stdout}
}

// cmdFails is the result of a command that fails with exit status 1 and
// stderr.
func cmdFails(stderr string) execstub.Result {
	return execstub.Result{Stderr: stderr, ExitCode: 1}
}

// gemVersionRe matches the version of a gem version file.
var gemVersionRe = regexp.MustCompile(`VERSION = "(.*)"`)

// expectGemBuild adds the commands of the gem build to s, where rake
// build gives build.
func expectGemBuild(s *cmdStubs, build execstub.Result) {
	s.expect(cmdOK(""), "bundle", "install")
	s.expect(build, "bundle", "exec", "rake", "build")
}

// rakeBuildWrites is the result of "bundle exec rake build" for the gem
// name with the version file versionFile: it writes the .gem file of
// the version and prints its name, as rake build does.
func rakeBuildWrites(name, versionFile string) execstub.Result {
	return execstub.Result{Func: func(_ context.Context, cmd *exec.Cmd, _ io.Reader, stdout, _ io.Writer) error {
		b, err := os.ReadFile(filepath.Join(cmd.Dir, versionFile))
		if err != nil {
			return fmt.Errorf("read the gem version: %w", err)
		}
		m := gemVersionRe.FindSubmatch(b)
		if m == nil {
			return fmt.Errorf("%w: %s", errGemNoVersion, versionFile)
		}
		pkg := filepath.Join(cmd.Dir, "pkg")
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			return fmt.Errorf("make %s: %w", pkg, err)
		}
		version := gemVersion(string(m[1]))
		gem := name + "-" + version + ".gem"
		if err := os.WriteFile(filepath.Join(pkg, gem), []byte("gem"), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", gem, err)
		}
		fmt.Fprintf(stdout, "%s %s built to pkg/%s.\n", name, version, gem)
		return nil
	}}
}
