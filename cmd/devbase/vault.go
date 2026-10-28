// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: vault CLI subcommand.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/vault"
	"github.com/urfave/cli/v3"
)

// vaultFormatJSON is the only output format "vault kv get" supports.
const vaultFormatJSON = "json"

var (
	// errVaultUnsupported is returned for any `devbase vault` command or
	// flag that is not implemented. It must be an error and never a
	// silent success: the `vault` CLI is being removed from CI images, so
	// a missing feature has to fail loudly for someone to add it.
	errVaultUnsupported = errors.New("not implemented by devbase vault (use the vault CLI, or add it to devbase vault)")

	// errVaultMissingCredentials is returned when the AppRole credentials
	// are not in the environment.
	errVaultMissingCredentials = errors.New("VAULT_ROLE_ID and VAULT_SECRET_ID must be set")

	// errVaultKVGetArgs is returned when "kv get" is not given exactly one path.
	errVaultKVGetArgs = errors.New("expected exactly one secret path")

	// errVaultFieldMissing is returned when "-field" names a key the secret lacks.
	errVaultFieldMissing = errors.New("field not present in secret")
)

// newVaultCommand returns the "vault" command: the small part of the
// `vault` CLI that devbase's CI scripts use, replacing the CLI there.
// Its commands and flags use the CLI's names and semantics, so a `vault`
// wrapper script can pass its arguments straight through, but it is not
// a general replacement: see errVaultUnsupported.
func newVaultCommand() *cli.Command {
	cmd := &cli.Command{
		Name:  "vault",
		Usage: "A minimal replacement for the vault CLI, for use in CI",
		Commands: []*cli.Command{
			newVaultLoginAppRoleCommand(),
			newVaultKVCommand(),
		},
		Action: vaultUnsupported,
	}
	setVaultUsageErrorHandler(cmd)
	return cmd
}

// setVaultUsageErrorHandler makes cmd, and every command under it,
// report a flag it does not define as errVaultUnsupported. By default
// cli/v3 says only "flag provided but not defined", which does not tell
// whoever sees it that devbase vault is what lacks the flag.
func setVaultUsageErrorHandler(cmd *cli.Command) {
	cmd.OnUsageError = func(_ context.Context, _ *cli.Command, err error, _ bool) error {
		return fmt.Errorf("%w: %w", errVaultUnsupported, err)
	}
	for _, sub := range cmd.Commands {
		setVaultUsageErrorHandler(sub)
	}
}

// vaultAddressFlag returns the "-address" flag, as the vault CLI has it.
// Without it, VAULT_ADDR is used.
func vaultAddressFlag() cli.Flag {
	return &cli.StringFlag{Name: "address", Usage: "Address of the Vault server (default: $VAULT_ADDR)"}
}

// newVaultLoginAppRoleCommand returns "vault login-approle". It is
// devbase's own command (the vault CLI has no such command), replacing
// `vault write auth/approle/login`. The credentials come from the
// environment, never from arguments, which other users can read.
func newVaultLoginAppRoleCommand() *cli.Command {
	return &cli.Command{
		Name: "login-approle",
		Usage: "Log in with the AppRole in $VAULT_ROLE_ID and $VAULT_SECRET_ID, " +
			"and write the token to ~/.vault-token",
		Flags:  []cli.Flag{vaultAddressFlag()},
		Action: runVaultLoginAppRole,
	}
}

// runVaultLoginAppRole is the Action for "vault login-approle".
func runVaultLoginAppRole(ctx context.Context, c *cli.Command) error {
	roleID, secretID := os.Getenv("VAULT_ROLE_ID"), os.Getenv("VAULT_SECRET_ID")
	if roleID == "" || secretID == "" {
		return errVaultMissingCredentials
	}

	client, err := vault.New(c.String("address"))
	if err != nil {
		return err
	}
	token, err := client.AppRoleLogin(ctx, roleID, secretID)
	if err != nil {
		return err
	}
	path, err := vault.WriteTokenFile(token)
	if err != nil {
		return err
	}

	// The token is deliberately not printed.
	fmt.Fprintf(c.ErrWriter, "Logged in to Vault with AppRole. The token is in %s\n", path)
	return nil
}

// newVaultKVCommand returns "vault kv", which only has "get".
func newVaultKVCommand() *cli.Command {
	return &cli.Command{
		Name:  "kv",
		Usage: "Interact with Vault's key-value secrets",
		Commands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "Read a secret",
				ArgsUsage: "<path>",
				Flags: []cli.Flag{
					vaultAddressFlag(),
					&cli.StringFlag{Name: "field", Usage: "Print only this field's value"},
					&cli.StringFlag{Name: "format", Usage: "Output format. Only \"json\" is supported"},
				},
				Action: runVaultKVGet,
			},
		},
		Action: vaultUnsupported,
	}
}

// runVaultKVGet is the Action for "vault kv get".
func runVaultKVGet(ctx context.Context, c *cli.Command) error {
	if c.NArg() != 1 {
		return fmt.Errorf("%w: got %d", errVaultKVGetArgs, c.NArg())
	}
	field, format := c.String("field"), c.String("format")
	if format != "" && format != vaultFormatJSON {
		return fmt.Errorf("-format=%s: %w", format, errVaultUnsupported)
	}
	if field == "" && format == "" {
		return fmt.Errorf("the default table output: %w; use -format=json or -field", errVaultUnsupported)
	}

	client, err := vault.New(c.String("address"))
	if err != nil {
		return err
	}
	secret, err := client.KVGet(ctx, c.Args().First())
	if err != nil {
		return err
	}

	// Like the vault CLI, -field wins over -format.
	if field != "" {
		return printVaultField(c.Writer, secret, field)
	}
	enc := json.NewEncoder(c.Writer)
	enc.SetIndent("", "  ")
	return enc.Encode(secret.Response)
}

// printVaultField prints the value of one field of secret. Like the
// vault CLI, it ends the value with a newline only on a terminal, so
// that `$(...)` and redirects get the exact value.
func printVaultField(w io.Writer, secret *vault.Secret, field string) error {
	value, ok := secret.Data[field]
	if !ok {
		return fmt.Errorf("%q: %w", field, errVaultFieldMissing)
	}

	out, isString := value.(string)
	if !isString {
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode field %q: %w", field, err)
		}
		out = string(b)
	}
	if isTerminal(w) {
		out += "\n"
	}
	_, err := io.WriteString(w, out)
	return err
}

// isTerminal reports whether w is a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// vaultUnsupported is the Action of every `vault` command that only
// groups others. cli/v3 runs it when no subcommand matches, so an
// unknown command fails here instead of printing help and exiting 0.
func vaultUnsupported(_ context.Context, c *cli.Command) error {
	command := strings.TrimSpace(c.FullName() + " " + strings.Join(c.Args().Slice(), " "))
	return fmt.Errorf("%q: %w", command, errVaultUnsupported)
}
