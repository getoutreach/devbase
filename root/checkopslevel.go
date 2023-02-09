//go:build mage

package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/getoutreach/devbase/v2/root/opslevel"
)

// CheckOpsLevel checks if the app passes all the required OpsLevel checks.
func CheckOpsLevel(ctx context.Context, appName string) error {
	client, err := opslevel.NewClient(opslevel.DefaultLifecycleToLevelMap)
	if err != nil {
		return errors.Wrap(err, "creating opslevel client")
	}

	serviceId, err := client.GetServiceIdWithAlias(appName)
	if err != nil {
		return errors.Wrap(err, "getting service id")
	}

	service, err := client.GetService(serviceId.Id)
	if err != nil {
		return errors.Wrap(err, "getting service")
	}

	sm, err := client.GetServiceMaturityWithAlias(appName)
	if err != nil {
		return errors.Wrap(err, "getting maturity reports")
	}

	isCompliant, err := client.IsCompliant(service, sm)
	if err != nil {
		return errors.Wrap(err, "checking compliance")
	}

	if isCompliant {
		return nil
	}

	return fmt.Errorf("project is not compliant")
}
