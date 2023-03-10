// Copyright 2023 Outreach Corporation. All Rights Reserved.

// Description: This file implements helpers for working with the OpsLevel API.

// Package opslevel implements helpers for working with the OpsLevel API.
package opslevel

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/opslevel/opslevel-go/v2022"
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

// DefaultLifecycleToLevelMap maps lifecycle index to level index.
// We want to keep this at the index level in case names or other attributes change.
var DefaultLifecycleToLevelMap = LifecycleToLevelMap{
	DevelopmentLifecycle: BronzeLevel,
	PrivateBetaLifecycle: SilverLevel,
	PublicBetaLifecycle:  SilverLevel,
	PublicLifecycle:      SilverLevel,
	OpsLifecycle:         SilverLevel,
	EndOfLifeLifecycle:   BeginnerLevel,
}

// LifecycleToLevelMap maps between Lifecycle and expected service maturity level
type LifecycleToLevelMap map[int]int

// Client is a opslevel client
type Client struct {
	*opslevel.Client // Internal opslevel client.

	// LifecycleToLevelMap maps lifecycle index to level index.
	LifecycleToLevelMap LifecycleToLevelMap
}

// NewClient returns a new opslevel client configured with the token we expect to exist in
// the environment.
func NewClient(lifecycleToLevelMap LifecycleToLevelMap) (*Client, error) {
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

// IsCompliant checks if the service falls within the expected maturity level.
// This check is primarily controlled by the LifecycleToLevel map
func (c Client) IsCompliant(service *opslevel.Service, sm *opslevel.ServiceMaturity) (bool, error) {
	if ShouldBeSkipped(service.Tags.Nodes) {
		return true, nil
	}

	// lifecycle index is zero with empty name when lifecycle is not assigned in opslevel
	if service.Lifecycle.Name == "" {
		return false, fmt.Errorf("no lifecycle assigned to %q", service.Name)
	}

	currentLevelIndex := sm.MaturityReport.OverallLevel.Index
	if len(c.LifecycleToLevelMap) < service.Lifecycle.Index {
		return false, fmt.Errorf("unsupported lifecycle %d %s",
			service.Lifecycle.Index, service.Lifecycle.Name)
	}

	expectedLevelIndex := c.LifecycleToLevelMap[service.Lifecycle.Index]
	return currentLevelIndex >= expectedLevelIndex, nil
}

// ShouldBeSkipped checks if a service has gating disabled. If it does, it should be skipped.
func ShouldBeSkipped(tags []opslevel.Tag) bool {
	// This is an internal tag that we use to skip gating.
	skipKey := "gating_disabled"
	for _, tag := range tags {
		if tag.Key == skipKey && tag.Value == "true" {
			return true
		}
	}

	return false
}

// GetExpectedLevel retrieves the expected maturity level of the service
func (c Client) GetExpectedLevel(service *opslevel.Service, levels []opslevel.Level) (string, error) {
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
