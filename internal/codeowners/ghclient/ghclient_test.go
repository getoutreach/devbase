// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the GitHub API client of the CODEOWNERS owners check.

package ghclient

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// newPrivateKey returns a PEM-encoded RSA private key for a GitHub App.
func newPrivateKey(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NilError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func TestConfigValidate(t *testing.T) {
	key := []byte("key")
	tests := map[string]struct {
		cfg  Config
		want error
	}{
		"access token":       {cfg: Config{AccessToken: "tok"}},
		"GitHub App":         {cfg: Config{AppID: 1, AppInstallationID: 2, AppPrivateKey: key}},
		"no authorization":   {cfg: Config{}, want: ErrNoAuth},
		"token and App":      {cfg: Config{AccessToken: "tok", AppID: 1, AppInstallationID: 2, AppPrivateKey: key}, want: ErrTokenAndApp},
		"no installation ID": {cfg: Config{AppID: 1, AppPrivateKey: key}, want: ErrNoInstallationID},
		"no private key":     {cfg: Config{AppID: 1, AppInstallationID: 2}, want: ErrNoPrivateKey},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.want == nil {
				assert.NilError(t, err)
				return
			}
			assert.Assert(t, errors.Is(err, tc.want), "got %v", err)
		})
	}
}

func TestNewInvalidConfig(t *testing.T) {
	_, err := New(Config{})
	assert.Assert(t, errors.Is(err, ErrNoAuth), "got %v", err)
}

func TestNewInvalidPrivateKey(t *testing.T) {
	_, err := New(Config{AppID: 1, AppInstallationID: 2, AppPrivateKey: []byte("not a key")})
	assert.ErrorContains(t, err, "GitHub App")
}

func TestNewAccessToken(t *testing.T) {
	gh, err := New(Config{AccessToken: "tok", Timeout: 5 * time.Second})
	assert.NilError(t, err)

	assert.Equal(t, gh.BaseURL.String(), "https://api.github.com/")
	assert.Equal(t, gh.Client().Timeout, 5*time.Second)
	assert.Equal(t, http.DefaultClient.Timeout, time.Duration(0), "http.DefaultClient must not change")
}

func TestNewEnterpriseURLs(t *testing.T) {
	tests := map[string]struct {
		cfg        Config
		wantUpload string
	}{
		"upload URL defaults to the base URL": {
			cfg:        Config{AccessToken: "tok", BaseURL: "https://ghe.example.com//"},
			wantUpload: "https://ghe.example.com/api/uploads/",
		},
		"explicit upload URL": {
			cfg:        Config{AccessToken: "tok", BaseURL: "https://ghe.example.com", UploadURL: "https://upload.example.com"},
			wantUpload: "https://upload.example.com/api/uploads/",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gh, err := New(tc.cfg)
			assert.NilError(t, err)
			assert.Equal(t, gh.BaseURL.String(), "https://ghe.example.com/api/v3/")
			assert.Equal(t, gh.UploadURL.String(), tc.wantUpload)
		})
	}
}

func TestNewAccessTokenSendsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, cmp.Equal(r.URL.Path, "/api/v3/repos/org/repo"))
		assert.Check(t, cmp.Equal(r.Header.Get("Authorization"), "Bearer tok"))
		_, _ = w.Write([]byte(`{"name":"repo"}`))
	}))
	t.Cleanup(srv.Close)

	gh, err := New(Config{AccessToken: "tok", BaseURL: srv.URL})
	assert.NilError(t, err)

	_, _, err = gh.Repositories.Get(t.Context(), "org", "repo")
	assert.NilError(t, err)
}

// TestNewAppUsesBaseURL checks that a GitHub App gets its installation token from the
// enterprise API URL, not from api.github.com.
func TestNewAppUsesBaseURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v3/app/installations/42/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "), "no App JWT")
		w.WriteHeader(http.StatusCreated)
		assert.Check(t, json.NewEncoder(w).Encode(map[string]any{
			"token":      "installation-token",
			"expires_at": time.Now().Add(time.Hour),
		}))
	})
	mux.HandleFunc("GET /api/v3/repos/org/repo", func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, cmp.Equal(r.Header.Get("Authorization"), "token installation-token"))
		_, _ = w.Write([]byte(`{"name":"repo"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	gh, err := New(Config{
		AppID:             7,
		AppInstallationID: 42,
		AppPrivateKey:     newPrivateKey(t),
		BaseURL:           srv.URL,
		Timeout:           5 * time.Second,
	})
	assert.NilError(t, err)
	assert.Equal(t, gh.Client().Timeout, 5*time.Second)

	_, _, err = gh.Repositories.Get(t.Context(), "org", "repo")
	assert.NilError(t, err)
}
