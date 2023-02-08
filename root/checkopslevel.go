//go:build mage

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"

	opslevel "github.com/opslevel/opslevel-go/v2022"
)

// Initializes constants for OpsLevel level and lifecycle indexes.
const (
	// BeginnerLevel is the index for the Beginner level in OpsLevel.
	BeginnerLevel = 0
	// BronzeLevel is the index for Bronze level in  OpsLevel.
	BronzeLevel = 1
	// SilverLevel is the index for Silver level in  OpsLevel.
	SilverLevel = 2
	// SilverUpcomingLevel is the index for the Silver (Upcoming) level in OpsLevel.
	SilverUpcomingLevel = 3
	// GoldLevel is the index for Gold level in  OpsLevel.
	GoldLevel = 4
	// PlatinumLevel is the index for Platinum level in  OpsLevel.
	PlatinumLevel = 5

	// DevelopmentLifecycle if the index for the Development lifecycle in OpsLevel.
	DevelopmentLifecycle = 1
	// PrivateBetaLifecycle if the index for the Private Beta lifecycle in OpsLevel.
	PrivateBetaLifecycle = 2
	// PublicBetaLifecycle if the index for the Public Beta lifecycle in OpsLevel.
	PublicBetaLifecycle = 3
	// PublicLifecycle if the index for the Public lifecycle in OpsLevel.
	PublicLifecycle = 4
	// OpsLifecycle if the index for the Ops lifecycle in OpsLevel.
	OpsLifecycle = 5
	// EndOfLifeLifecycle if the index for the Endo-of-Life lifecycle in OpsLevel.
	EndOfLifeLifecycle = 6
)

// LifecycleToLevel maps lifecycle index to level index.
// We want to keep this at the index level in case names or other attributes change.
var DefaultLifecycleToLevelMap = LifecycleToLevelMap{
	DevelopmentLifecycle: BronzeLevel,
	PrivateBetaLifecycle: SilverLevel,
	PublicBetaLifecycle:  SilverLevel,
	PublicLifecycle:      SilverLevel,
	OpsLifecycle:         SilverLevel,
	EndOfLifeLifecycle:   BeginnerLevel,
}

// CheckOpsLevel checks if the app passes all the required OpsLevel checks.
func CheckOpsLevel(ctx context.Context, appName string) error {
	client, err := newClient(DefaultLifecycleToLevelMap)
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

	isCompliant, err := client.isCompliant(service, sm)
	if err != nil {
		return errors.Wrap(err, "checking compliance")
	}

	if isCompliant {
		return nil
	}

	return fmt.Errorf("project is not compliant")
}

// LifecycleToLevelMap maps between Lifecycle and expected service maturity level
type LifecycleToLevelMap map[int]int

// Client is a opslevel client
type Client struct {
	*opslevel.Client // Internal opslevel client.

	// LifecycleToLevelMap maps lifecycle index to level index.
	LifecycleToLevelMap LifecycleToLevelMap
}

// newClient returns a new opslevel client configured with the token we expect to exist in
// the environment.
func newClient(lifecycleToLevelMap LifecycleToLevelMap) (*Client, error) {
	opslevelToken := strings.TrimSpace(os.Getenv("OPSLEVEL_TOKEN"))
	if opslevelToken == "" {
		return nil, errors.New("OPSLEVEL_TOKEN environment variable is empty")
	}
	if len(lifecycleToLevelMap) == 0 {
		lifecycleToLevelMap = DefaultLifecycleToLevelMap
	}
	return &Client{
		opslevel.NewGQLClient(opslevel.SetAPIToken(opslevelToken)),
		lifecycleToLevelMap,
	}, nil
}

// isCompliant checks if the service falls within the expected maturity level.
// This check is primarily controlled by the LifecycleToLevel map
func (c Client) isCompliant(service *opslevel.Service, sm *opslevel.ServiceMaturity) (bool, error) {
	if shouldBeSkipped(service.Tags.Nodes) {
		return true, nil
	}

	currentLevelIndex := sm.MaturityReport.OverallLevel.Index
	if len(c.LifecycleToLevelMap) < service.Lifecycle.Index {
		return false, fmt.Errorf("unsupported lifecycle %d %s",
			service.Lifecycle.Index, service.Lifecycle.Name)
	}

	expectedLevelIndex := c.LifecycleToLevelMap[service.Lifecycle.Index]
	return currentLevelIndex >= expectedLevelIndex, nil
}

// shouldBeSkipped checks if a service has gating disabled. If it does, it should be skipped.
func shouldBeSkipped(tags []opslevel.Tag) bool {
	// This is an internal tag that we use to skip gating.
	skipKey := "gating_disabled"
	for _, tag := range tags {
		if tag.Key == skipKey && tag.Value == "true" {
			return true
		}
	}

	return false
}

// getExpectedLevel retrieves the expected maturity level of the service
func (c Client) getExpectedLevel(service *opslevel.Service, levels []opslevel.Level) (string, error) {
	if len(c.LifecycleToLevelMap) < service.Lifecycle.Index {
		return "", fmt.Errorf("unsupported lifecycle %d %s",
			service.Lifecycle.Index, service.Lifecycle.Name)
	}
	expectedLevelIndex := c.LifecycleToLevelMap[service.Lifecycle.Index]

	for _, l := range levels {
		if l.Index == expectedLevelIndex {
			return l.Name, nil
		}
	}

	return "", fmt.Errorf("unable to find level index %d", expectedLevelIndex)
}
