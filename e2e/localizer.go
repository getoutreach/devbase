// Copyright 2023 Outreach Corporation. All Rights Reserved.

// Description: This file contains the code to run localizer for devenv

package main

import (
	"context"
	"os/exec"
	"time"

	"github.com/getoutreach/gobox/pkg/async"
	localizerapi "github.com/getoutreach/localizer/api"
	"github.com/getoutreach/localizer/pkg/localizer"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ensureRunningLocalizerWorks check if a localizer is already running, and if it is
// ensure it's working properly (responding to pings). If it's not, remove the socket.
func ensureRunningLocalizerWorks(ctx context.Context) error {
	log.Info().Msg("Ensuring existing localizer is actually running")
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	client, closer, err := localizer.Connect(ctx,
		grpc.WithBlock(), //nolint:staticcheck // Why: This is deprecated but supported until grpc 2.0.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	// Made connection, ping it
	if err == nil {
		defer closer()

		// Responding to pings, return nil
		if _, err := client.Ping(ctx, &localizerapi.PingRequest{}); err == nil {
			return nil
		}
	}

	// not responding to pings, or failed to connect, remove the socket
	//nolint:gosec // Why: We're OK with this. It's a constant.
	return osStdInOutErr(exec.Command("sudo", "rm", "-f", localizer.Socket)).Run()
}

// runLocalizer runs localizer for devenv
func runLocalizer(ctx context.Context) (cleanup func(), err error) {
	if localizer.IsRunning() {
		if err := ensureRunningLocalizerWorks(ctx); err != nil {
			return nil, err
		}
	}

	if !localizer.IsRunning() {
		// Preemptively ask for sudo to prevent input mangling with o.LocalApps
		log.Info().Msg("You may get a sudo prompt so localizer can create tunnels")
		if err := osStdInOutErr(exec.CommandContext(ctx, "sudo", "true")).Run(); err != nil {
			log.Fatal().Err(err).Msg("Failed to get root permissions")
		}

		log.Info().Msg("Starting devenv tunnel")
		if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "tunnel")).Start(); err != nil {
			log.Fatal().Err(err).Msg("Failed to start devenv tunnel")
		}

		// Wait until localizer is running, max 1m
		//nolint:govet // Why: done on purpose
		ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Minute))
		defer cancel()

		for ctx.Err() == nil && !localizer.IsRunning() {
			async.Sleep(ctx, time.Second*1)
		}
	}

	client, closer, err := localizer.Connect(ctx,
		grpc.WithBlock(), //nolint:staticcheck // Why: This is deprecated but supported until grpc 2.0.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to localizer")
	}
	defer closer()

	log.Info().Msg("Waiting for devenv tunnel to be finished creating tunnels")
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Minute))
	defer cancel()

	for ctx.Err() == nil {
		resp, err := client.Stable(ctx, &localizerapi.Empty{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to check if localizer is running")
		}

		if resp.Stable {
			break
		}

		async.Sleep(ctx, time.Second*2)
	}

	return func() {
		log.Info().Msg("Killing the spawned localizer process (spawned by devenv tunnel)")
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := client.Kill(ctx, &localizerapi.Empty{}); err != nil {
			log.Warn().Err(err).Msg("failed to kill running localizer server")
		}
	}, nil
}
