//go:build mage

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/getoutreach/gobox/pkg/box"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	logger "github.com/rs/zerolog/log"
)

// Deploy pushs new actionable version to maestro for given app and channel
func Deploy(ctx context.Context, appName, channel string) error {
	log := logger.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	appVersion := getAppVersion()

	log.Info().Msgf("Triggering deployment for %s version=%q channel=%q", appName, appVersion, channel)

	conf, err := box.LoadBox()
	if err != nil {
		return errors.Wrap(err, "failed to read box config")
	}

	url := fmt.Sprintf("%s/applications/%s/deploymentSegments/%s/actionableVersion", conf.CD.Maestro.Address, appName, channel)
	reqPayload := map[string]string{
		"version": appVersion,
	}

	encoded, err := json.Marshal(reqPayload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request payload")
	}
	req, err := retryablehttp.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return errors.Wrap(err, "failed to new request")
	}
	req.Header.Add("X-Auth-Token", os.Getenv("OUTREACH_MAESTRO_SECRET"))

	resp, err := retryablehttp.NewClient().Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}

	defer resp.Body.Close()

	respPayload, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read response payload")
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code, [%d] %s", resp.StatusCode, string(respPayload))
	}

	log.Info().Msgf("Successfully triggered deployment, %s", respPayload)

	return nil
}
