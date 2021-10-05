#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run clerkgenproto(https://github.com/getoutreach/clerkgen/tree/main/cmd/clerkgenproto)

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# Get GH_TOKEN
if [[ -n $OUTREACH_GITHUB_TOKEN ]]; then
	GH_TOKEN="$OUTREACH_GITHUB_TOKEN"
elif [[ -f "$HOME/.outreach/github.token" ]]; then
	GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
else
	echo "" >/dev/stderr
    echo "Unable to find Github personal access token. This is required by clerkgen" >/dev/stderr
    exit 1
fi

# Verify GH_TOKEN
resp=$(curl -s -o /dev/null -I -w "%{http_code}" -H "Authorization: token $GH_TOKEN" https://api.github.com/orgs/getoutreach/repos)
if [[ $resp -ne 200 ]]; then
    echo "Unable to run clerkgen. Github personal access is not valid." >/dev/stderr
	exit 1
fi

# Verify docker is running
docker info > /dev/null 2>/dev/stderr
exit_code=$(echo $?)
if [[ $exit_code != "0" ]]; then
    echo "Unable to run clerkgen. Docker is not running. Please start docker." >/dev/stderr
    exit "$exit_code"
fi

#Verify AWS creds
aws sts get-caller-identity > /dev/null 2>/dev/stderr
exit_code=$(echo $?)
if [[ $exit_code != "0" ]]; then
    echo "Unable to run clerkgen. AWS credentials are not valid. Please run 'saml2aws login'." >/dev/stderr
    exit "$exit_code"
fi

# Run clerkgen
"$GOBIN" "github.com/getoutreach/clerkgen/cmd/clerkgenproto@v$(get_application_version "clerkgenproto")" "$@"