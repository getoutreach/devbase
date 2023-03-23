//go:build mage

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	logger "github.com/rs/zerolog/log"

	"github.com/getoutreach/devbase/v2/root/opslevel"
)

// CheckOpsLevel checks if the app passes all the required OpsLevel checks.
func CheckOpsLevel(ctx context.Context, appName string) error {
	log := logger.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	client, err := opslevel.NewClient(opslevel.DefaultLifecycleToLevelMap)
	if err != nil {
		return errors.Wrap(err, "failed to create opslevel client")
	}

	log.Info().Msgf("Retrieving opslevel service entry of %q", appName)

	serviceId, err := client.GetServiceIdWithAlias(appName)
	if err != nil {
		return errors.Wrap(err, "failed to get service id")
	}

	if serviceId == nil || serviceId.Id == nil {
		return errors.New("unable to find opslevel service entry")
	}

	service, err := client.GetService(serviceId.Id)
	if err != nil {
		return errors.Wrap(err, "failed to get service entry")
	}

	log.Info().Msgf("Retrieving opslevel service maturity reports of %q", appName)

	sm, err := client.GetServiceMaturityWithAlias(appName)
	if err != nil {
		return errors.Wrap(err, "failed to get maturity reports")
	}

	if sm == nil || sm.MaturityReport.OverallLevel.Name == "" {
		return errors.New("unable to find opslevel service maturity reports")
	}

	isCompliant, err := client.IsCompliant(service, sm)
	if err != nil {
		return errors.Wrap(err, "failed to check compliance")
	}

	// If the app is compliant, return nil
	if isCompliant {
		return nil
	}

	levels, err := client.ListLevels()
	if err != nil {
		return errors.Wrap(err, "failed to list levels")
	}

	expectedLevel, err := client.GetExpectedLevel(service, levels)
	if err != nil {
		return errors.Wrap(err, "failed to get expected level")
	}

	return fmt.Errorf("The service maturity level of %q in OpsLevel does not match the required level for the lifecycle: %q (current), %q (expected)", service.Name, sm.MaturityReport.OverallLevel.Name, expectedLevel)
}
