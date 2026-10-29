// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: A minimal Vault HTTP API client: AppRole login and KV reads.

// Package vault is a minimal client for the parts of the Vault HTTP API
// that devbase's CI scripts use: AppRole login and KV (v1 and v2)
// reads. It replaces the `vault` CLI there, so the CLI does not have to
// be installed in CI images.
//
// It is not a general Vault client. Anything it does not implement is an
// error, never a silent no-op, so callers find the missing feature
// instead of getting wrong results.
package vault

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// These are the errors that this package returns. Each is wrapped with
// context where it is returned.
var (
	// errAddressNotSet is returned when no Vault address is configured.
	errAddressNotSet = errors.New("vault address is not set (use -address or VAULT_ADDR)")

	// errNoToken is returned when a request needs a token and none is available.
	errNoToken = errors.New("no vault token (set VAULT_TOKEN or log in so that ~/.vault-token exists)")

	// errNoClientToken is returned when a login response has no client token.
	errNoClientToken = errors.New("vault login response has no client token")

	// errInvalidCACert is returned when VAULT_CACERT has no usable certificates.
	errInvalidCACert = errors.New("VAULT_CACERT has no valid PEM certificates")

	// errNoMountInfo is returned when the lookup of a path's mount has no mount path in it.
	errNoMountInfo = errors.New("vault returned no mount path")

	// errNoValue is returned when a secret has no data, as when its latest KV v2 version is deleted.
	errNoValue = errors.New("no value found at path")
)

// These constants set the token file's name and mode, and the limits for
// requests to Vault.
const (
	// tokenFileName is the file in the home directory that the `vault`
	// CLI, and many scripts, read and write the token from.
	tokenFileName = ".vault-token"

	// tokenFileMode is the mode of the token file. It holds a credential.
	tokenFileMode = 0o600

	// requestTimeout bounds a single HTTP attempt.
	requestTimeout = 30 * time.Second

	// maxRetries is how many times a failed request is retried. It matches
	// the Vault API client's default.
	maxRetries = 2

	// maxBodyBytes bounds how much of a response body is read.
	maxBodyBytes = 10 << 20
)

// APIError is an error response from the Vault server.
type APIError struct {
	// Method and Path identify the request. Path never contains a token.
	Method string
	Path   string

	// StatusCode is the HTTP status code of the response.
	StatusCode int

	// Messages are the errors in the response body, if it had any.
	Messages []string
}

// Error implements error.
func (e *APIError) Error() string {
	msg := strings.Join(e.Messages, "; ")
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("vault %s %s: %d: %s", e.Method, e.Path, e.StatusCode, msg)
}

// Secret is the result of reading a secret.
type Secret struct {
	// Response is the full response body from Vault, the same JSON that
	// `vault kv get -format=json` prints.
	Response map[string]any

	// Data is the secret's key/value pairs. For KV v2 it is the nested
	// "data" object of the response's "data".
	Data map[string]any
}

// Client talks to one Vault server.
type Client struct {
	addr      string
	token     string
	namespace string
	hc        *retryablehttp.Client
}

// New returns a Client for addr, or for VAULT_ADDR if addr is empty. The
// token comes from VAULT_TOKEN, or else from ~/.vault-token if that file
// exists. It also honors VAULT_NAMESPACE and VAULT_CACERT.
func New(addr string) (*Client, error) {
	if addr == "" {
		addr = os.Getenv("VAULT_ADDR")
	}
	if addr == "" {
		return nil, errAddressNotSet
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	// VAULT_CACERT is set by whoever runs the process, as with the vault CLI.
	// The file is only parsed as PEM certificates, and its content is never printed.
	if caFile := os.Getenv("VAULT_CACERT"); caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read VAULT_CACERT: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("%s: %w", caFile, errInvalidCACert)
		}
		tlsCfg.RootCAs = pool
	}

	hc := retryablehttp.NewClient()
	hc.Logger = nil
	hc.RetryMax = maxRetries
	// Return the last response, not an error, when retries run out, so
	// callers can report the status code and message.
	hc.ErrorHandler = retryablehttp.PassthroughErrorHandler
	// Timeout bounds a whole attempt. The transport sets its limits too, so that
	// it never waits without one.
	hc.HTTPClient = &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSClientConfig:       tlsCfg,
			TLSHandshakeTimeout:   requestTimeout,
			ResponseHeaderTimeout: requestTimeout,
		},
	}

	c := &Client{
		addr:      strings.TrimRight(addr, "/"),
		token:     os.Getenv("VAULT_TOKEN"),
		namespace: os.Getenv("VAULT_NAMESPACE"),
		hc:        hc,
	}
	if c.token == "" {
		c.token = readTokenFile()
	}
	return c, nil
}

// AppRoleLogin logs in with an AppRole role ID and secret ID, uses the
// resulting token for later requests, and returns it.
func (c *Client) AppRoleLogin(ctx context.Context, roleID, secretID string) (string, error) {
	var out struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	body := map[string]string{"role_id": roleID, "secret_id": secretID}
	// A login request has no token, even if one is set already.
	if err := c.do(ctx, http.MethodPost, "auth/approle/login", "", body, &out); err != nil {
		return "", fmt.Errorf("approle login: %w", err)
	}
	if out.Auth.ClientToken == "" {
		return "", errNoClientToken
	}
	c.token = out.Auth.ClientToken
	return c.token, nil
}

// KVGet reads the secret at path, which includes the mount, for example
// "deploy/app/key". It supports KV v1 and v2, like `vault kv get`.
func (c *Client) KVGet(ctx context.Context, path string) (*Secret, error) {
	if c.token == "" {
		return nil, errNoToken
	}
	path = strings.Trim(path, "/")

	mountPath, v2, err := c.kvMount(ctx, path)
	if err != nil {
		return nil, err
	}

	reqPath := path
	if v2 {
		reqPath = mountPath + "data/" + strings.TrimPrefix(path, mountPath)
	}

	var resp map[string]any
	if err := c.do(ctx, http.MethodGet, escapePath(reqPath), c.token, nil, &resp); err != nil {
		return nil, err
	}

	data, _ := resp["data"].(map[string]any)
	if v2 {
		data, _ = data["data"].(map[string]any)
	}
	if data == nil {
		return nil, fmt.Errorf("%s: %w", path, errNoValue)
	}
	return &Secret{Response: resp, Data: data}, nil
}

// kvMount returns the mount path (with a trailing slash) that path is
// under, and whether that mount is KV v2. It uses the same lookup as the
// `vault` CLI. A server that does not have it (404) is treated as KV v1.
func (c *Client) kvMount(ctx context.Context, path string) (mountPath string, v2 bool, err error) {
	var out struct {
		Data struct {
			Path    string            `json:"path"`
			Options map[string]string `json:"options"`
		} `json:"data"`
	}
	err = c.do(ctx, http.MethodGet, "sys/internal/ui/mounts/"+escapePath(path), c.token, nil, &out)

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if out.Data.Path == "" {
		return "", false, fmt.Errorf("%s: %w", path, errNoMountInfo)
	}
	return out.Data.Path, out.Data.Options["version"] == "2", nil
}

// do sends a request to the Vault API and decodes the JSON response into
// out. path is relative to "/v1/" and must already be escaped.
func (c *Client) do(ctx context.Context, method, path, token string, body, out any) error {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		payload = bytes.NewReader(b)
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, method, c.addr+"/v1/"+path, payload)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("vault %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("vault %s %s: read response: %w", method, path, err)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return newAPIError(method, path, resp.StatusCode, data)
	}
	if len(data) == 0 {
		return nil
	}
	// Keep numbers as written: a float64 rounds an integer above 2^53.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("vault %s %s: decode response: %w", method, path, err)
	}
	return nil
}

// newAPIError returns an APIError for a response with an error status. The
// messages are empty if the body is not Vault's JSON error format, for
// example an HTML page from a proxy.
func newAPIError(method, path string, status int, body []byte) *APIError {
	var parsed struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		// The body is not always JSON (a proxy can return HTML), so a
		// parse failure just means there are no messages.
		parsed.Errors = nil
	}
	return &APIError{Method: method, Path: path, StatusCode: status, Messages: parsed.Errors}
}

// escapePath escapes each segment of a "/"-separated path, keeping the
// separators. url.PathEscape alone would escape them too.
func escapePath(p string) string {
	segments := strings.Split(p, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

func tokenFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, tokenFileName), nil
}

// WriteTokenFile writes token to the token file, so that later steps and
// tools (including the `vault` CLI) can use it, and returns the file's
// path. The file is readable by the owner only, even if it existed with
// other permissions.
func WriteTokenFile(token string) (string, error) {
	p, err := tokenFilePath()
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, tokenFileMode)
	if err != nil {
		return "", fmt.Errorf("open token file: %w", err)
	}
	defer f.Close()

	// OpenFile does not change the mode of a file that already exists, and
	// has emptied it. Change the mode before the token is written.
	if err := f.Chmod(tokenFileMode); err != nil {
		return "", fmt.Errorf("set token file mode: %w", err)
	}
	if _, err := f.WriteString(token); err != nil {
		return "", fmt.Errorf("write token file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close token file: %w", err)
	}
	return p, nil
}

// readTokenFile returns the token in the token file, or "" if it cannot
// be read, so that the lack of a token is reported where one is needed.
func readTokenFile() string {
	p, err := tokenFilePath()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
