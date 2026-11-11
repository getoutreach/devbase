// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Runs the registry CLIs (npm, circleci) for the registry
// publishers, through a runner that tests can replace.

package release

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// secureMask replaces a secret in logs and errors, as semantic-release
// masks secrets.
const secureMask = "[secure]"

// regCommand is a command of a registry CLI.
type regCommand struct {
	// dir is the working directory.
	dir string

	// env is the environment.
	env []string

	// name is the program, and args are its arguments.
	name string
	args []string

	// secrets are values in args that logs and errors show as
	// secureMask.
	secrets []string
}

// String returns the command line with the secrets masked.
func (c *regCommand) String() string {
	return maskSecrets(strings.Join(append([]string{c.name}, c.args...), " "), c.secrets)
}

// regResult is the output of a registry command.
type regResult struct {
	stdout string
	stderr string
}

// regRunner runs a registry command. It writes the command line, with
// the secrets masked, and the command's output to log, and returns the
// output. A command that exits with an error returns its output and an
// error.
type regRunner func(ctx context.Context, log io.Writer, c *regCommand) (regResult, error)

// execRunner is the regRunner of production: it runs the command with
// os/exec.
func execRunner(ctx context.Context, log io.Writer, c *regCommand) (regResult, error) {
	fmt.Fprintf(log, "Run %s\n", c)
	cmd := exec.CommandContext(ctx, c.name, c.args...) //nolint:gosec // Why: the registry CLIs with plan values.
	var stdout, stderr bytes.Buffer
	cmd.Dir, cmd.Env = c.dir, c.env
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if len(c.secrets) == 0 {
		// The output streams to the log; with secrets, it goes there
		// masked when the command ends.
		cmd.Stdout, cmd.Stderr = io.MultiWriter(&stdout, log), io.MultiWriter(&stderr, log)
	}
	err := cmd.Run()
	res := regResult{stdout: stdout.String(), stderr: stderr.String()}
	if len(c.secrets) > 0 {
		io.WriteString(log, maskSecrets(res.stdout+res.stderr, c.secrets)) //nolint:errcheck // Why: best-effort log.
	}
	if err != nil {
		return res, fmt.Errorf("%s: %w: %s", c, err, maskSecrets(strings.TrimSpace(res.stderr), c.secrets))
	}
	return res, nil
}

// maskSecrets replaces each non-empty secret in s with secureMask.
func maskSecrets(s string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, secureMask)
		}
	}
	return s
}

// registryEnv returns the environment of the registry commands: the
// publish environment, or the process environment when it is nil, as
// os/exec does.
func registryEnv(p *regTarget) []string {
	if p.env == nil {
		return os.Environ()
	}
	return p.env
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
