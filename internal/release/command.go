// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Runs the child processes of the release: git, go, exec
// hooks, the registry CLIs and the unstable include script, with one
// rule for logs, secret masking and errors.

package release

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/getoutreach/devbase/v2/internal/release/rlog"
)

// command is a child process of the release.
type command struct {
	// dir is the working directory.
	dir string

	// env is the environment. Nil means the process environment.
	env []string

	// name is the program, and args are its arguments.
	name string
	args []string

	// display is what the log shows for the command. Empty means the
	// program and its arguments.
	display string
}

// String returns the command line, or display.
func (c *command) String() string {
	if c.display != "" {
		return c.display
	}
	return strings.Join(append([]string{c.name}, c.args...), " ")
}

// cmdResult is the output of a command.
type cmdResult struct {
	stdout string
	stderr string
}

// cmdRunner is the runner of the registry CLIs, which tests replace.
type cmdRunner func(ctx context.Context, log io.Writer, c *command) (cmdResult, error)

// execCommand runs a command with os/exec. The command line and the
// output go to log while it runs, and a failed command returns an error
// with the command line, stderr and stdout. The log and the error have
// the secrets masked, as semantic-release masks its output, with the
// Masker of ctx, or else of the command's environment.
func execCommand(ctx context.Context, log io.Writer, c *command) (cmdResult, error) {
	m := maskerFor(ctx, c.env)
	cmd := exec.CommandContext(ctx, c.name, c.args...) //nolint:gosec // Why: the release runs the commands of its config.
	var stdout, stderr bytes.Buffer
	cmd.Dir, cmd.Env = c.dir, c.env
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if log != io.Discard {
		// os/exec copies stdout and stderr in two goroutines, and both
		// write to log.
		log = &syncWriter{w: m.Writer(log)}
		fmt.Fprintf(log, "Run %s\n", c)
		cmd.Stdout, cmd.Stderr = io.MultiWriter(&stdout, log), io.MultiWriter(&stderr, log)
	}
	err := cmd.Run()
	res := cmdResult{stdout: stdout.String(), stderr: stderr.String()}
	if err != nil {
		out := strings.TrimSpace(strings.TrimSpace(res.stderr) + "\n" + strings.TrimSpace(res.stdout))
		return res, fmt.Errorf("%s: %w: %s", m.Mask(c.String()), err, m.Mask(out))
	}
	return res, nil
}

// syncWriter serializes the writes to w.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write writes p to w while it holds the lock.
func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// maskerFor returns the Masker of ctx, or else a Masker of env.
func maskerFor(ctx context.Context, env []string) *rlog.Masker {
	if m := rlog.MaskerFrom(ctx); m != nil {
		return m
	}
	return rlog.NewMasker(envOrProcess(env))
}

// run runs a command in dir that logs only to the debug log, such as git
// and go, and returns its stdout without the surrounding whitespace.
func run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	c := &command{dir: dir, env: env, name: name, args: args}
	rlog.Debugf(ctx, "run %s", c)
	res, err := execCommand(ctx, io.Discard, c)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.stdout), nil
}

// envOrProcess returns env, or the process environment when env is nil,
// as os/exec does.
func envOrProcess(env []string) []string {
	if env == nil {
		return os.Environ()
	}
	return env
}

// envValue returns the value of key in env, or "". The last entry wins,
// as in os/exec.
func envValue(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}

// withoutEnv returns env without the entries of key.
func withoutEnv(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); k != key {
			out = append(out, kv)
		}
	}
	return out
}
