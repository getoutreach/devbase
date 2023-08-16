#!/usr/bin/env bash
# Runs the delibird log/tracing/session-recording uploader if
# configured to do so in the box configuration.
#
# Automatically handles installation of the delibird log uploader if it
# is not already installed.
set -euo pipefail

# shellcheck source=../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"
# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"
# shellcheck source=../../lib/github.sh
source "$DIR/../../lib/github.sh"

# DELIBIRD_ENABLED denotes if the delibird log uploader should be
# enabled or not. If the value is "true", then the delibird log uploader
# will be enabled. If the value is "false", then the delibird log
# uploader will be disabled.
DELIBIRD_ENABLED=$(get_box_field ".delibird.enabled")

# install_delibird installs the delibird log uploader.
install_delibird() {
  # We enable pre-releases for now because we rely on the latest
  # unstable version of delibird to function.
  install_latest_github_release getoutreach/orc true delibird
}

# Exit if we're not enabled.
if [[ $DELIBIRD_ENABLED != "true" ]]; then
  exit 0
fi

# Otherwise, check if the uploader is installed. If it isn't, attempt to
# install it.
if ! command -v delibird &>/dev/null; then
  install_delibird
fi

info "Running the delibird log uploader"
exec delibird --run-once start
