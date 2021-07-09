#!/usr/bin/env bash
#
# This script uploads various build stats to OpsLevel.
# At first, our plan is to upload Prisma Cloud image scan results
# (vulnerabilities and compliance issues), more to come later.
#

# We only upload to opslevel on tagged releases.
if [[ -z $CIRCLECI ]]; then
  echo "OpsLevel upload can only run in CircleCI"
  exit 1
fi
