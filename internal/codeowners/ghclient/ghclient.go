// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Creating the GitHub API client that the CODEOWNERS owners check uses.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

// Package ghclient creates a GitHub API client that authenticates with an access token or as a
// GitHub App installation.
package ghclient

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v84/github"
)

// The errors that Config.Validate returns.
var (
	// ErrNoAuth is returned when the configuration has neither an access token nor a GitHub App ID.
	ErrNoAuth = errors.New("GitHub authorization is required: provide an access token or a GitHub App ID")

	// ErrTokenAndApp is returned when the configuration has both an access token and a GitHub App ID.
	ErrTokenAndApp = errors.New("an access token cannot be used together with a GitHub App ID")

	// ErrNoInstallationID is returned when the configuration has a GitHub App ID but no installation ID.
	ErrNoInstallationID = errors.New("a GitHub App installation ID is required with a GitHub App ID")

	// ErrNoPrivateKey is returned when the configuration has a GitHub App ID but no private key.
	ErrNoPrivateKey = errors.New("a GitHub App private key is required with a GitHub App ID")
)

// Config configures the GitHub API client. It needs either an access token, or a GitHub App ID
// with an installation ID and a private key.
type Config struct {
	// AccessToken is a classic or fine-grained personal access token, or an OAuth token.
	AccessToken string

	// AppID is the ID of the GitHub App to authenticate as.
	AppID int64

	// AppInstallationID is the ID of the installation of the GitHub App.
	AppInstallationID int64

	// AppPrivateKey is the PEM-encoded private key of the GitHub App.
	AppPrivateKey []byte

	// BaseURL is the URL of a GitHub Enterprise Server. The client uses api.github.com when it is empty.
	BaseURL string

	// UploadURL is the upload URL of a GitHub Enterprise Server. It defaults to BaseURL.
	UploadURL string

	// Timeout limits the time of each HTTP request. Zero means no limit.
	Timeout time.Duration
}

// Validate returns an error when the configuration does not describe exactly one way to authenticate.
//
//nolint:gocritic // Why: hugeParam: a Config is validated once, so copying 96 bytes does not matter.
func (c Config) Validate() error {
	switch {
	case c.AccessToken == "" && c.AppID == 0:
		return ErrNoAuth
	case c.AccessToken != "" && c.AppID != 0:
		return ErrTokenAndApp
	case c.AppID != 0 && c.AppInstallationID == 0:
		return ErrNoInstallationID
	case c.AppID != 0 && len(c.AppPrivateKey) == 0:
		return ErrNoPrivateKey
	default:
		return nil
	}
}

// New returns a GitHub API client for cfg.
//
//nolint:gocritic // Why: hugeParam: a client is created once, so copying 96 bytes does not matter.
func New(cfg Config) (*github.Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: cfg.Timeout}

	var appTransport *ghinstallation.Transport
	if cfg.AppID != 0 {
		var err error
		appTransport, err = ghinstallation.New(http.DefaultTransport, cfg.AppID, cfg.AppInstallationID, cfg.AppPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("create the GitHub App installation transport: %w", err)
		}
		httpClient.Transport = appTransport
	}

	gh := github.NewClient(httpClient)
	if cfg.AccessToken != "" {
		gh = gh.WithAuthToken(cfg.AccessToken)
	}

	if cfg.BaseURL == "" {
		return gh, nil
	}

	// Extra trailing slashes would end up in the API paths.
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	uploadURL := cmp.Or(strings.TrimRight(cfg.UploadURL, "/"), baseURL)

	gh, err := gh.WithEnterpriseURLs(baseURL, uploadURL)
	if err != nil {
		return nil, fmt.Errorf("set the GitHub Enterprise URLs: %w", err)
	}

	if appTransport != nil {
		// The App gets its installation tokens from the API, so it must use the enterprise API URL.
		appTransport.BaseURL = strings.TrimSuffix(gh.BaseURL.String(), "/")
	}

	return gh, nil
}
