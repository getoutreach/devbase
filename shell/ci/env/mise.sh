#!/usr/bin/env bash
# Setup mise for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -e

# TODO(malept): feature parity with asdf.sh in the same folder.
mise install --yes
