#!/usr/bin/env bash
# debug.sh - Debug a Go application.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# PACKAGE_TO_DEBUG is the package to debug. If not set, it will default to the
# main package, which is cmd/<app_name>.
PACKAGE_TO_DEBUG="${PACKAGE_TO_DEBUG:-$(get_repo_directory)/cmd/$(get_app_name)}"

# IN_CONTAINER is a flag that indicates whether or not we are running in a
# container or not. If not set, will be determined automatically.
#
# Why: Declared for documentation/discoverability purposes.
# shellcheck disable=SC2269
IN_CONTAINER="${IN_CONTAINER}"

# Determine if we're running in a container or not.
if [[ -z $IN_CONTAINER ]]; then
  # DLV_PORT is the port that DLV will listen on and is set by the devcontainer.
  # KUBERNETES_SERVICE_HOST is set automatically when running in Kubernetes, which
  # is a good indicator that we're running in a container.
  if [[ -z ${DLV_PORT} ]] && [[ -z $KUBERNETES_SERVICE_HOST ]]; then
    IN_CONTAINER=false
  else
    IN_CONTAINER=true
  fi
fi

# HEADLESS is a flag that indicates whether or not we should run the debugger in
# headless mode. If not set, it will be enabled if we're in a container. Otherwise,
# it will be disabled.
#
# Running in headless mode starts delve in a mode where it will wait for a
# debugger to connect before starting the application.
HEADLESS="${HEADLESS:-$IN_CONTAINER}"

# DLV_PORT is the port that DLV will listen on when running in headless mode.
#
# Why: Declared for documentation/discoverability purposes.
# shellcheck disable=SC2269
DLV_PORT="${DLV_PORT}"
if [[ $HEADLESS == "true" ]] && [[ -z $DLV_PORT ]]; then
  echo "DLV_PORT must be set when running in headless mode" >&2
  exit 1
fi
DLV_VERSION="v$(get_application_version "delve")"

# DEV_CONTAINER_LOGFILE is the path to the log file that will be used for all output
# from the debugger. This is only used when running in a container and ideally would be
# be used for propagating logs to a logging system.
DEV_CONTAINER_LOGFILE=${DEV_CONTAINER_LOGFILE:-$TMPDIR/app.log}

echo "Starting debugger for package '$PACKAGE_TO_DEBUG' (headless: $HEADLESS)" >&2
if [[ $HEADLESS == "true" ]]; then
  echo "Headless Information:" >&2
  echo "  - DLV_PORT: $DLV_PORT" >&2
  echo "  - DEV_CONTAINER_LOGFILE: $DEV_CONTAINER_LOGFILE" >&2

  mkdir -p "$(dirname "$DEV_CONTAINER_LOGFILE")" 2>/dev/null >&2 || true
fi
echo

delve_path=$("$DIR/gobin.sh" -p github.com/go-delve/delve/cmd/dlv@"$DLV_VERSION")

# delve is the command that will be executed to start the debugger
delve=(
  "$delve_path"
  debug
  --build-flags="-tags=or_dev"
  "$PACKAGE_TO_DEBUG"
)

# Set flags for the debugger when running in headless mode
if [[ $HEADLESS == "true" ]]; then
  delve+=(
    --headless
    --api-version=2
    --listen=":${DLV_PORT}"
  )
fi

function ctrl_c_trap() {
  echo "killing delve with PID: $DLV_PID"
  kill "$DLV_PID"
  exit
}

trap ctrl_c_trap SIGINT

# When not running in a container, we can start without logging.
# Otherwise, we need to log to a file so that the output can be
# processed by devspace.
if [[ $IN_CONTAINER == "false" ]]; then
  exec "${delve[@]}"
else
  echo -e "\n\n\n\n\n\n\n\n" >>"$DEV_CONTAINER_LOGFILE"

  # Start delve in the background so we can kill it on ctrl-c.
  "${delve[@]}" >>"$DEV_CONTAINER_LOGFILE" 2>&1 &
  DLV_PID=$!
  echo "delve pid is: $DLV_PID"

  # tail to watch logs here
  tail -n 5 -f "$DEV_CONTAINER_LOGFILE"
fi

wait
