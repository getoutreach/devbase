// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the Vault HTTP API client.

package vault

import (
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// isolateEnv points HOME at an empty temp directory and clears the
// variables New reads, so a test does not see the developer's own Vault
// setup. It returns the temp HOME.
func isolateEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"VAULT_ADDR", "VAULT_TOKEN", "VAULT_NAMESPACE", "VAULT_CACERT"} {
		t.Setenv(k, "")
	}
	return home
}

// newClient returns a Client for srv with no retry delay.
func newClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(srv.URL)
	assert.NilError(t, err)
	c.hc.RetryWaitMin = time.Millisecond
	c.hc.RetryWaitMax = time.Millisecond
	return c
}

// writeJSON writes v as a JSON response.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	assert.Check(t, json.NewEncoder(w).Encode(v))
}

func TestNew_Address(t *testing.T) {
	isolateEnv(t)

	_, err := New("")
	assert.Assert(t, errors.Is(err, errAddressNotSet), err)

	t.Setenv("VAULT_ADDR", "https://vault.example.com/")
	c, err := New("")
	assert.NilError(t, err)
	assert.Equal(t, c.addr, "https://vault.example.com")

	c, err = New("https://other.example.com")
	assert.NilError(t, err)
	assert.Equal(t, c.addr, "https://other.example.com")
}

func TestNew_Token(t *testing.T) {
	home := isolateEnv(t)

	c, err := New("https://vault.example.com")
	assert.NilError(t, err)
	assert.Equal(t, c.token, "")

	assert.NilError(t, os.WriteFile(filepath.Join(home, ".vault-token"), []byte("from-file\n"), 0o600))
	c, err = New("https://vault.example.com")
	assert.NilError(t, err)
	assert.Equal(t, c.token, "from-file")

	t.Setenv("VAULT_TOKEN", "from-env")
	c, err = New("https://vault.example.com")
	assert.NilError(t, err)
	assert.Equal(t, c.token, "from-env")
}

func TestAppRoleLogin(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "should-not-be-sent")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, cmp.Equal(r.Method, http.MethodPost))
		assert.Check(t, cmp.Equal(r.URL.Path, "/v1/auth/approle/login"))
		assert.Check(t, cmp.Equal(r.Header.Get("X-Vault-Token"), ""))

		var body map[string]string
		assert.Check(t, json.NewDecoder(r.Body).Decode(&body))
		if body["role_id"] != "role" || body["secret_id"] != "secret" {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(t, w, map[string]any{"errors": []string{"invalid role or secret ID"}})
			return
		}
		writeJSON(t, w, map[string]any{"auth": map[string]any{"client_token": "s.abc"}})
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv)

	_, err := c.AppRoleLogin(t.Context(), "role", "wrong")
	var apiErr *APIError
	assert.Assert(t, errors.As(err, &apiErr), err)
	assert.Equal(t, apiErr.StatusCode, http.StatusBadRequest)
	assert.ErrorContains(t, err, "invalid role or secret ID")

	token, err := c.AppRoleLogin(t.Context(), "role", "secret")
	assert.NilError(t, err)
	assert.Equal(t, token, "s.abc")
	assert.Equal(t, c.token, "s.abc")
}

func TestAppRoleLogin_NoClientToken(t *testing.T) {
	isolateEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"auth": nil})
	}))
	t.Cleanup(srv.Close)

	_, err := newClient(t, srv).AppRoleLogin(t.Context(), "role", "secret")
	assert.Assert(t, errors.Is(err, errNoClientToken), err)
}

// kvServer serves a KV mount at mount, of the given version, holding the
// secret rest -> data. It requires the token "tok".
func kvServer(t *testing.T, mount, version, rest string, data map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			writeJSON(t, w, map[string]any{"errors": []string{"permission denied"}})
			return
		}
		switch r.URL.Path {
		case "/v1/sys/internal/ui/mounts/" + mount + rest:
			writeJSON(t, w, map[string]any{
				"data": map[string]any{"path": mount, "options": map[string]string{"version": version}},
			})
		case "/v1/" + mount + "data/" + rest:
			assert.Check(t, cmp.Equal(version, "2"))
			writeJSON(t, w, map[string]any{"data": map[string]any{"data": data, "metadata": map[string]any{"version": 3}}})
		case "/v1/" + mount + rest:
			assert.Check(t, cmp.Equal(version, "1"))
			writeJSON(t, w, map[string]any{"data": data})
		default:
			w.WriteHeader(http.StatusNotFound)
			writeJSON(t, w, map[string]any{"errors": []string{}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestKVGet(t *testing.T) {
	tests := []struct {
		name    string
		mount   string
		version string
		rest    string
		path    string
	}{
		{name: "v2", mount: "deploy/", version: "2", rest: "delibird/development/upload", path: "deploy/delibird/development/upload"},
		{name: "v2 nested mount", mount: "secret/team/", version: "2", rest: "app", path: "secret/team/app"},
		{name: "v2 leading and trailing slash", mount: "deploy/", version: "2", rest: "a/b", path: "/deploy/a/b/"},
		{name: "v1", mount: "legacy/", version: "1", rest: "app/key", path: "legacy/app/key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			t.Setenv("VAULT_TOKEN", "tok")
			srv := kvServer(t, tt.mount, tt.version, tt.rest, map[string]any{"token": "abc", "n": 1})
			c := newClient(t, srv)

			secret, err := c.KVGet(t.Context(), tt.path)
			assert.NilError(t, err)
			assert.Equal(t, secret.Data["token"], "abc")
			assert.Equal(t, secret.Data["n"], json.Number("1"))
			assert.Assert(t, secret.Response["data"] != nil)
		})
	}
}

func TestKVGet_MountLookupNotFoundMeansV1(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/legacy/app" {
			writeJSON(t, w, map[string]any{"data": map[string]any{"k": "v"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	secret, err := newClient(t, srv).KVGet(t.Context(), "legacy/app")
	assert.NilError(t, err)
	assert.Equal(t, secret.Data["k"], "v")
}

func TestKVGet_SpecialCharacters(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")
	srv := kvServer(t, "deploy/", "2", "my app/a%b", map[string]any{"k": "v"})

	// The "/" separators stay; characters that need escaping are escaped.
	assert.Equal(t, escapePath("deploy/my app/a%b"), "deploy/my%20app/a%25b")

	secret, err := newClient(t, srv).KVGet(t.Context(), "deploy/my app/a%b")
	assert.NilError(t, err)
	assert.Equal(t, secret.Data["k"], "v")
}

func TestKVGet_Errors(t *testing.T) {
	isolateEnv(t)

	srv := kvServer(t, "deploy/", "2", "app", map[string]any{})
	c := newClient(t, srv)

	_, err := c.KVGet(t.Context(), "deploy/app")
	assert.Assert(t, errors.Is(err, errNoToken), err)

	c.token = "wrong"
	_, err = c.KVGet(t.Context(), "deploy/app")
	var apiErr *APIError
	assert.Assert(t, errors.As(err, &apiErr), err)
	assert.Equal(t, apiErr.StatusCode, http.StatusForbidden)
	assert.ErrorContains(t, err, "permission denied")

	c.token = "tok"
	_, err = c.KVGet(t.Context(), "deploy/missing")
	assert.Assert(t, errors.As(err, &apiErr), err)
	assert.Equal(t, apiErr.StatusCode, http.StatusNotFound)
}

func TestDo_RetriesServerErrors(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/internal/ui/mounts/legacy/app" && calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/v1/legacy/app" {
			writeJSON(t, w, map[string]any{"data": map[string]any{"k": "v"}})
			return
		}
		writeJSON(t, w, map[string]any{"data": map[string]any{"path": "legacy/", "options": map[string]string{"version": "1"}}})
	}))
	t.Cleanup(srv.Close)

	secret, err := newClient(t, srv).KVGet(t.Context(), "legacy/app")
	assert.NilError(t, err)
	assert.Equal(t, secret.Data["k"], "v")
	assert.Equal(t, calls.Load(), int32(3))
}

// TestDo_EmptyResponse confirms a success with no body, such as a 204, is
// not a decode error, and leaves what it was to decode into as it was.
func TestDo_EmptyResponse(t *testing.T) {
	isolateEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv)

	var out map[string]any
	assert.NilError(t, c.do(t.Context(), http.MethodGet, "sys/anything", "", nil, &out))
	assert.Assert(t, out == nil)

	_, err := c.AppRoleLogin(t.Context(), "role", "secret")
	assert.Assert(t, errors.Is(err, errNoClientToken), err)
}

// TestKVGet_KeepsNumbers confirms a number is not rounded to a float64, as
// an integer above 2^53 would be.
func TestKVGet_KeepsNumbers(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")
	srv := kvServer(t, "legacy/", "1", "app", map[string]any{"id": int64(9007199254740993)})

	secret, err := newClient(t, srv).KVGet(t.Context(), "legacy/app")
	assert.NilError(t, err)
	assert.Equal(t, secret.Data["id"], json.Number("9007199254740993"))
}

// TestKVGet_NothingReturned confirms a response with no mount path, or no
// data, is an error and not a guess or an empty secret.
func TestKVGet_NothingReturned(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")

	for name, tt := range map[string]struct {
		mount   func(w http.ResponseWriter)
		wantErr error
	}{
		"no mount path": {
			mount:   func(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) },
			wantErr: errNoMountInfo,
		},
		"no data": {
			mount: func(w http.ResponseWriter) {
				writeJSON(t, w, map[string]any{"data": map[string]any{"path": "deploy/", "options": map[string]string{"version": "2"}}})
			},
			wantErr: errNoValue,
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/sys/internal/ui/mounts/") {
					tt.mount(w)
					return
				}
				// A KV v2 secret whose latest version is deleted has null data.
				writeJSON(t, w, map[string]any{"data": map[string]any{"data": nil}})
			}))
			t.Cleanup(srv.Close)

			_, err := newClient(t, srv).KVGet(t.Context(), "deploy/app")
			assert.Assert(t, errors.Is(err, tt.wantErr), err)
		})
	}
}

func TestDo_ReportsServerErrorAfterRetries(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	t.Cleanup(srv.Close)

	_, err := newClient(t, srv).KVGet(t.Context(), "deploy/app")
	var apiErr *APIError
	assert.Assert(t, errors.As(err, &apiErr), err)
	assert.Equal(t, apiErr.StatusCode, http.StatusBadGateway)
	assert.ErrorContains(t, err, "Bad Gateway")
	assert.Equal(t, calls.Load(), int32(maxRetries+1))
}

func TestDo_Namespace(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")
	t.Setenv("VAULT_NAMESPACE", "team-a")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Header.Get("X-Vault-Namespace"), "team-a")
		writeJSON(t, w, map[string]any{"data": map[string]any{"path": "legacy/", "options": nil}})
	}))
	t.Cleanup(srv.Close)

	_, err := newClient(t, srv).KVGet(t.Context(), "legacy/app")
	assert.NilError(t, err)
}

func TestNew_CACert(t *testing.T) {
	isolateEnv(t)
	t.Setenv("VAULT_TOKEN", "tok")

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"data": map[string]any{"path": "legacy/", "options": nil}})
	}))
	// The failed handshake below is expected; keep it out of the test log.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	t.Cleanup(srv.Close)

	// Without the server's certificate, TLS verification fails.
	_, err := newClient(t, srv).KVGet(t.Context(), "legacy/app")
	assert.Assert(t, err != nil)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	assert.NilError(t, os.WriteFile(caFile, pemBytes, 0o600))
	t.Setenv("VAULT_CACERT", caFile)

	_, err = newClient(t, srv).KVGet(t.Context(), "legacy/app")
	assert.NilError(t, err)

	assert.NilError(t, os.WriteFile(caFile, []byte("not a certificate"), 0o600))
	_, err = New(srv.URL)
	assert.Assert(t, errors.Is(err, errInvalidCACert), err)
}

func TestWriteTokenFile(t *testing.T) {
	home := isolateEnv(t)
	p := filepath.Join(home, ".vault-token")

	// An existing file with wider permissions is tightened.
	assert.NilError(t, os.WriteFile(p, []byte("old"), 0o644))
	got, err := WriteTokenFile("s.new")
	assert.NilError(t, err)
	assert.Equal(t, got, p)

	data, err := os.ReadFile(p)
	assert.NilError(t, err)
	assert.Equal(t, string(data), "s.new")

	info, err := os.Stat(p)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
}
