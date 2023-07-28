#!/usr/bin/env bash
# debug.sh - Debug a Go application.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# DEV_CONTAINER_EXECUTABLE is set by Devspace here: https://github.com/getoutreach/stencil-golang/blob/cb950f2fc050bb112492626d70c772dc15ffc4ae/templates/devspace.yaml.tpl#LL19C17-L19C17
# If it is set, use it, otherwise use the application name.
DEV_CONTAINER_EXECUTABLE="${DEV_CONTAINER_EXECUTABLE:-$(get_app_name)}"

# PACKAGE_TO_DEBUG is the package to debug. If not set, it will default to the
# to `./cmd/$DEV_CONTAINER_EXECUTABLE`
PACKAGE_TO_DEBUG="${PACKAGE_TO_DEBUG:-$(get_repo_directory)/cmd/${DEV_CONTAINER_EXECUTABLE}}"

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

  if [[ $IN_CONTAINER == "true" ]]; then
    echo "  - DEV_CONTAINER_LOGFILE: $DEV_CONTAINER_LOGFILE" >&2
    mkdir -p "$(dirname "$DEV_CONTAINER_LOGFILE")" 2>/dev/null >&2 || true
  fi
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
  exit 0
}
trap ctrl_c_trap SIGINT

if [[ $HEADLESS == "false" ]]; then
  exec "${delve[@]}"
else

  # Start headless delve in the background so we can kill it with ctrl-c.

  if [[ $IN_CONTAINER == "false" ]]; then
    # no need to log outside of a container
    "${delve[@]}" &
  else
    # We only need to start logging if we are running in a
    # a container so the output can be processed by devspace.
    echo -e "\n\n\n\n\n\n\n\n" >>"$DEV_CONTAINER_LOGFILE"
    "${delve[@]}" >>"$DEV_CONTAINER_LOGFILE" 2>&1 &
  fi

  DLV_PID=$!
  echo "delve pid is: $DLV_PID"

  if [[ $IN_CONTAINER != "false" ]]; then
    # tail to watch logs here
    tail -n 5 -f "$DEV_CONTAINER_LOGFILE"
  fi
fi

wait
