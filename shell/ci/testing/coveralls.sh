#!/usr/bin/env bash
# Uploads code coverage to coveralls.io

if [[ -n $COVERALLS_TOKEN ]]; then
  goveralls -coverprofile=/tmp/coverage.out -service=circle-ci -repotoken="$COVERALLS_TOKEN"
fi
