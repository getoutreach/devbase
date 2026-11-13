// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the shared command runner.

package release

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// TestRun checks that run logs only to the debug log, as git and go
// commands do, returns the trimmed stdout, and has stderr and stdout in
// its error.
func TestRun(t *testing.T) {
	var debug bytes.Buffer
	ctx := rlog.WithDebug(context.Background(), &debug)
	out, err := run(ctx, t.TempDir(), nil, "/bin/sh", "-c", "echo '  hello  '")
	assert.NilError(t, err)
	assert.Equal(t, out, "hello")
	assert.Equal(t, debug.String(), "debug: run /bin/sh -c echo '  hello  '\n")

	_, err = run(context.Background(), t.TempDir(), nil, "/bin/sh", "-c", "echo out; echo err >&2; exit 2")
	assert.ErrorContains(t, err, "/bin/sh -c echo out; echo err >&2; exit 2: exit status 2: err\nout")
}

// TestExecCommandMasksSecrets checks that execCommand masks the secrets
// of the command's environment in its log and error, as semantic-release
// masks its output, when the context has no Masker.
func TestExecCommandMasksSecrets(t *testing.T) {
	var log bytes.Buffer
	c := &command{
		dir: t.TempDir(), env: []string{"PATH=/usr/bin:/bin", "SOME_TOKEN=tok-SECRET"}, name: "/bin/sh",
		args: []string{"-c", `echo "bad token $SOME_TOKEN" >&2; exit 3`, "sh", "tok-SECRET"},
	}
	res, err := execCommand(context.Background(), nil, &log, c)
	assert.ErrorContains(t, err, "bad token [secure]")
	assert.Assert(t, !strings.Contains(err.Error(), "tok-SECRET"), err)
	assert.Equal(t, res.stderr, "bad token tok-SECRET\n")
	assert.Assert(t, strings.HasPrefix(log.String(), "Run /bin/sh -c"), log.String())
	assert.Assert(t, strings.Contains(log.String(), "sh [secure]\nbad token [secure]\n"), log.String())
	assert.Assert(t, !strings.Contains(log.String(), "tok-SECRET"), log.String())

	// The Masker of the context replaces the one of the environment.
	log.Reset()
	ctx := rlog.WithMasker(context.Background(), rlog.NewMasker([]string{"OTHER_TOKEN=bad token"}))
	_, err = execCommand(ctx, nil, &log, c)
	assert.ErrorContains(t, err, "[secure] tok-SECRET")
}

// TestExecCommandExec checks that execCommand creates the command with
// its Exec, in the dir and env of the command, with the same log and
// error as for a real process.
func TestExecCommandExec(t *testing.T) {
	cmds := newCmdStubs(t)
	cmds.expect(execstub.Result{Stdout: "out tok-SECRET\n", Stderr: "err\n", ExitCode: 2}, "tool", "--token", "tok-SECRET")
	var log bytes.Buffer
	c := &command{dir: "/repo", env: []string{"SOME_TOKEN=tok-SECRET"}, name: "tool", args: []string{"--token", "tok-SECRET"}}
	res, err := execCommand(context.Background(), cmds.spy, &log, c)
	assert.Error(t, err, "tool --token [secure]: exit status 2: err\nout [secure]")
	assert.Equal(t, res.stdout, "out tok-SECRET\n")
	assert.Equal(t, res.stderr, "err\n")
	assert.Equal(t, log.String(), "Run tool --token [secure]\nout [secure]\nerr\n")
	cmd := cmds.calls()[0].Cmd
	assert.Equal(t, cmd.Dir, "/repo")
	assert.DeepEqual(t, cmd.Env, c.env)
}

// TestExecCommandConcurrentOutput checks that stdout and stderr, which
// os/exec copies in two goroutines, write to the log one at a time. Run
// it with -race.
func TestExecCommandConcurrentOutput(t *testing.T) {
	var log bytes.Buffer
	c := &command{
		dir: t.TempDir(), env: []string{"PATH=/usr/bin:/bin"}, name: "/bin/sh",
		args: []string{"-c", `for i in $(seq 200); do echo "out $i"; echo "err $i" >&2; done`},
	}
	_, err := execCommand(context.Background(), nil, &log, c)
	assert.NilError(t, err)
	// The first log line is the command line, which also has "out ".
	assert.Equal(t, strings.Count(log.String(), "\nout "), 200)
	assert.Equal(t, strings.Count(log.String(), "\nerr "), 200)
}

// TestRunExecHook checks that a hook's stdout and stderr go to the log,
// as @semantic-release/exec pipes them, and that its error has stderr.
func TestRunExecHook(t *testing.T) {
	var log bytes.Buffer
	fields := &spec.ExecFields{NextVersion: "1.2.3"}
	out, err := runExecHook(context.Background(), t.TempDir(), nil, &log,
		"echo '  v${nextRelease.version}  '; echo note >&2", fields)
	assert.NilError(t, err)
	assert.Equal(t, out, "v1.2.3")
	// stdout and stderr come from two pipes, so their order can change.
	assert.Assert(t, strings.HasPrefix(log.String(), "Run echo '  v1.2.3  '; echo note >&2\n"), log.String())
	assert.Assert(t, strings.Contains(log.String(), "  v1.2.3  \n"), log.String())
	assert.Assert(t, strings.Contains(log.String(), "note\n"), log.String())

	_, err = runExecHook(context.Background(), t.TempDir(), nil, &log, "echo broken >&2; exit 4", fields)
	assert.ErrorContains(t, err, "echo broken >&2; exit 4: exit status 4: broken")
}

// TestRunUnstableInclude checks the include script: it gets the process
// environment when env is nil, and DRYRUN=false.
func TestRunUnstableInclude(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, runUnstableInclude(context.Background(), dir, nil, &bytes.Buffer{}), "no script")

	t.Setenv("INCLUDE_TEST", "from-process")
	writeFile(t, dir, unstableInclude, "#!/bin/sh\necho \"$INCLUDE_TEST $DRYRUN\"\n")
	assert.NilError(t, os.Chmod(filepath.Join(dir, unstableInclude), 0o755))
	var log bytes.Buffer
	assert.NilError(t, runUnstableInclude(context.Background(), dir, nil, &log))
	assert.Equal(t, log.String(), "Run "+unstableInclude+"\nfrom-process false\n")
	assert.Assert(t, !strings.Contains(log.String(), "Calling"))
}
