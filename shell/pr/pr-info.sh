#!/usr/bin/env bash
# Gets info about the PR from GitHub

if [[ -z $CIRCLE_PULL_REQUEST ]]; then
  exit 0
fi

GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
if [[ -z $GH_TOKEN ]]; then
  echo "Failed to read Github personal access token" >&2
fi

echo -n "🔨 Getting PR info"
PR_NUMBER=${CIRCLE_PULL_REQUEST//[!0-9]/}
RESPONSE=$(
  curl --silent \
    -H "Authorization: token $GH_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/${CIRCLE_PROJECT_USERNAME}/${CIRCLE_PROJECT_REPONAME}/pulls/${PR_NUMBER}"
)

echo "$RESPONSE" >/tmp/pr_info.json

DRAFT=$(echo "$RESPONSE" | jq ".draft")
DRAFT_FILE="/tmp/pr_draft.txt"

if [[ $DRAFT == 'true' ]]; then
  touch $DRAFT_FILE
else
  rm -f $DRAFT_FILE
fi

TITLE=$(echo "$RESPONSE" | jq ".title")
TITLE_FILE="/tmp/pr_title.txt"
echo "$TITLE" >$TITLE_FILE

echo "OK"