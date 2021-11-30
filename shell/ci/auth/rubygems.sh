#!/usr/bin/env bash
# Sets up authentication for pushing up a RubyGem
# DEPRECATED: This is no longer supported and will be replaced
# by Github Packages very soon.
if [[ -z $PACKAGECLOUD_TOKEN ]]; then
  echo "Skipped: PACKAGECLOUD_TOKEN is not set"
fi
