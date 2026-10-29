// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: End-to-end tests for the "vault" CLI subcommand.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// runVaultCommand runs the "vault" subcommand like runYqCommand does for
// "yq".
func runVaultCommand(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runSubcommand(t, newVaultCommand(), "", args...)
}

// vaultTestServer serves a KV v2 mount "deploy/" holding one secret,
// deploy/app, and an AppRole login. It returns its URL, and sets HOME to
// an empty directory (returned) and VAULT_TOKEN, so the test uses neither
// the developer's Vault login nor its address.
func vaultTestServer(t *testing.T) (url, home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("VAULT_TOKEN", "tok")
	for _, k := range []string{"VAULT_ADDR", "VAULT_NAMESPACE", "VAULT_CACERT", "VAULT_ROLE_ID", "VAULT_SECRET_ID"} {
		t.Setenv(k, "")
	}

	write := func(w http.ResponseWriter, v any) {
		assert.Check(t, json.NewEncoder(w).Encode(v))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := r.Header.Get("X-Vault-Token"); r.URL.Path != "/v1/auth/approle/login" && tok != "tok" && tok != "s.logged-in" {
			w.WriteHeader(http.StatusForbidden)
			write(w, map[string]any{"errors": []string{"permission denied"}})
			return
		}
		switch r.URL.Path {
		case "/v1/auth/approle/login":
			var body map[string]string
			assert.Check(t, json.NewDecoder(r.Body).Decode(&body))
			if body["role_id"] != "role" || body["secret_id"] != "secret" {
				w.WriteHeader(http.StatusBadRequest)
				write(w, map[string]any{"errors": []string{"invalid secret id"}})
				return
			}
			write(w, map[string]any{"auth": map[string]any{"client_token": "s.logged-in"}})
		case "/v1/sys/internal/ui/mounts/deploy/app":
			write(w, map[string]any{"data": map[string]any{"path": "deploy/", "options": map[string]string{"version": "2"}}})
		case "/v1/deploy/data/app":
			write(w, map[string]any{"data": map[string]any{
				"data": map[string]any{
					"token": "abc", "count": 3, "big": int64(9007199254740993), "ok": true, "empty": nil,
					"nested": map[string]any{"k": "v"},
				},
				"metadata": map[string]any{"version": 1},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, home
}

// TestVaultCommand_KVGetField confirms the flag forms the vault CLI
// accepts all work, so a `vault` wrapper can pass its arguments through,
// and that the value has no trailing newline when not on a terminal.
func TestVaultCommand_KVGetField(t *testing.T) {
	url, _ := vaultTestServer(t)

	for _, flags := range [][]string{
		{"-field", "token"},
		{"-field=token"},
		{"--field", "token"},
		{"--field=token"},
	} {
		// -address comes after the subcommand, like the vault CLI.
		args := append([]string{"kv", "get", "-address=" + url}, flags...)
		stdout, _, err := runVaultCommand(t, append(args, "deploy/app")...)
		assert.NilError(t, err, flags)
		assert.Equal(t, stdout, "abc", flags)
	}

	// A field that is not a string is printed as JSON.
	stdout, _, err := runVaultCommand(t, "kv", "get", "-address="+url, "-field=count", "deploy/app")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "3")
}

// TestVaultCommand_KVGetAddressFromEnv confirms VAULT_ADDR is used when
// -address is not given.
func TestVaultCommand_KVGetAddressFromEnv(t *testing.T) {
	url, _ := vaultTestServer(t)
	t.Setenv("VAULT_ADDR", url)

	stdout, _, err := runVaultCommand(t, "kv", "get", "-field=token", "deploy/app")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "abc")
}

// TestVaultCommand_KVGetJSON confirms -format=json prints the full
// response, so that `jq .data.data` works as it does on the vault CLI's.
func TestVaultCommand_KVGetJSON(t *testing.T) {
	url, _ := vaultTestServer(t)

	for _, format := range [][]string{{"-format=json"}, {"-format", "json"}, {"--format=json"}} {
		args := append([]string{"kv", "get", "-address=" + url}, format...)
		stdout, _, err := runVaultCommand(t, append(args, "deploy/app")...)
		assert.NilError(t, err, format)

		var got struct {
			Data struct {
				Data     map[string]any `json:"data"`
				Metadata map[string]any `json:"metadata"`
			} `json:"data"`
		}
		assert.NilError(t, json.Unmarshal([]byte(stdout), &got), stdout)
		assert.Equal(t, got.Data.Data["token"], "abc")
		assert.Equal(t, got.Data.Metadata["version"], float64(1))
	}

	// Like the vault CLI, a field is printed as JSON with -format=json.
	stdout, _, err := runVaultCommand(t, "kv", "get", "-address="+url, "-format=json", "-field=token", "deploy/app")
	assert.NilError(t, err)
	assert.Equal(t, stdout, `"abc"`)
}

// TestVaultCommand_KVGetErrors confirms bad input and server errors are
// errors.
func TestVaultCommand_KVGetErrors(t *testing.T) {
	url, _ := vaultTestServer(t)
	addr := "-address=" + url

	_, _, err := runVaultCommand(t, "kv", "get", addr, "-field=missing", "deploy/app")
	assert.ErrorIs(t, err, errVaultFieldMissing)

	_, _, err = runVaultCommand(t, "kv", "get", addr, "-field=token")
	assert.ErrorIs(t, err, errVaultKVGetArgs)
	_, _, err = runVaultCommand(t, "kv", "get", addr, "-field=token", "deploy/app", "deploy/other")
	assert.ErrorIs(t, err, errVaultKVGetArgs)

	_, _, err = runVaultCommand(t, "kv", "get", addr, "-field=token", "deploy/not-there")
	assert.ErrorContains(t, err, "404")

	t.Setenv("VAULT_TOKEN", "")
	_, _, err = runVaultCommand(t, "kv", "get", addr, "-field=token", "deploy/app")
	assert.ErrorContains(t, err, "no vault token")

	_, _, err = runVaultCommand(t, "kv", "get", "-field=token", "deploy/app")
	assert.ErrorContains(t, err, "address is not set")
}

// TestVaultCommand_Unsupported confirms anything devbase vault does not
// implement fails with errVaultUnsupported, not success. This is what
// makes the `vault` wrapper in CI images safe: a missing feature breaks
// the job so it gets added.
func TestVaultCommand_Unsupported(t *testing.T) {
	url, _ := vaultTestServer(t)
	addr := "-address=" + url

	for _, args := range [][]string{
		// Missing and unknown commands.
		{},
		{"kv"},
		{"kv", "list", "deploy/"},
		{"kv", "put", "deploy/app", "a=b"},
		{"token", "lookup"},
		{"write", "auth/approle/login", "role_id=r"},
		{"print", "token"},
		// Unimplemented output formats.
		{"kv", "get", addr, "deploy/app"},
		{"kv", "get", addr, "-format=yaml", "deploy/app"},
	} {
		stdout, _, err := runVaultCommand(t, args...)
		assert.ErrorIs(t, err, errVaultUnsupported, "%v", args)
		assert.Equal(t, stdout, "", "%v", args)
	}

	// Unknown flags are the same error, whatever the command, and say
	// which flag it is.
	for _, args := range [][]string{
		{"kv", "get", addr, "-mount=deploy", "-field=token", "app"},
		{"kv", "get", addr, "-version=2", "-field=token", "deploy/app"},
		{"login-approle", "-method=oidc"},
		{"login", "-method=oidc"},
		{"-help-me"},
	} {
		_, _, err := runVaultCommand(t, args...)
		assert.ErrorIs(t, err, errVaultUnsupported, "%v", args)
		assert.ErrorContains(t, err, "flag provided but not defined", "%v", args)
	}
}

// TestVaultCommand_LoginAppRole confirms login-approle writes the token
// to ~/.vault-token, readable by the owner only, and does not print it.
func TestVaultCommand_LoginAppRole(t *testing.T) {
	url, home := vaultTestServer(t)
	t.Setenv("VAULT_ROLE_ID", "role")
	t.Setenv("VAULT_SECRET_ID", "secret")
	t.Setenv("VAULT_TOKEN", "")

	tokenPath := filepath.Join(home, ".vault-token")
	assert.NilError(t, os.WriteFile(tokenPath, []byte("old"), 0o644))

	stdout, stderr, err := runVaultCommand(t, "login-approle", "-address="+url)
	assert.NilError(t, err)
	assert.Equal(t, stdout, "")
	assert.Assert(t, !strings.Contains(stderr, "s.logged-in"), stderr)
	assert.Assert(t, strings.Contains(stderr, tokenPath), stderr)

	got, err := os.ReadFile(tokenPath)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "s.logged-in")
	info, err := os.Stat(tokenPath)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))

	// The token it wrote is what the next command uses.
	t.Setenv("VAULT_ADDR", url)
	stdout, _, err = runVaultCommand(t, "kv", "get", "-field=token", "deploy/app")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "abc")
}

// TestVaultCommand_LoginAppRoleErrors confirms login-approle fails, and
// leaves any existing token alone, on missing credentials or a rejected
// login.
func TestVaultCommand_LoginAppRoleErrors(t *testing.T) {
	url, home := vaultTestServer(t)
	tokenPath := filepath.Join(home, ".vault-token")
	assert.NilError(t, os.WriteFile(tokenPath, []byte("old"), 0o600))

	_, _, err := runVaultCommand(t, "login-approle", "-address="+url)
	assert.ErrorIs(t, err, errVaultMissingCredentials)

	t.Setenv("VAULT_ROLE_ID", "role")
	t.Setenv("VAULT_SECRET_ID", "wrong")
	_, _, err = runVaultCommand(t, "login-approle", "-address="+url)
	assert.ErrorContains(t, err, "invalid secret id")

	got, err := os.ReadFile(tokenPath)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "old")
}

// TestVaultCommand_KVGetFieldTypes confirms how each type of field is printed,
// and that a field the vault CLI would not print as it is, is an error.
func TestVaultCommand_KVGetFieldTypes(t *testing.T) {
	url, _ := vaultTestServer(t)

	for _, tt := range []struct {
		field, format, want string
	}{
		{field: "count", want: "3"},
		{field: "ok", want: "true"},
		// A number is not rounded to a float64.
		{field: "big", want: "9007199254740993"},
		{field: "nested", format: "json", want: "{\n  \"k\": \"v\"\n}"},
		{field: "ok", format: "json", want: "true"},
	} {
		args := []string{"kv", "get", "-address=" + url, "-field=" + tt.field}
		if tt.format != "" {
			args = append(args, "-format="+tt.format)
		}
		stdout, _, err := runVaultCommand(t, append(args, "deploy/app")...)
		assert.NilError(t, err, tt.field)
		assert.Equal(t, stdout, tt.want, tt.field)
	}

	// A null field is missing.
	_, _, err := runVaultCommand(t, "kv", "get", "-address="+url, "-field=empty", "deploy/app")
	assert.ErrorIs(t, err, errVaultFieldMissing)

	// A map is not printed as Go prints it, or as JSON that the vault CLI would not print.
	_, _, err = runVaultCommand(t, "kv", "get", "-address="+url, "-field=nested", "deploy/app")
	assert.ErrorIs(t, err, errVaultUnsupported)
}
