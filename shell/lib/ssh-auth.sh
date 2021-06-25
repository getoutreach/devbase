#!/usr/bin/env bash

echo "🔒 Setting up ssh access"
# Setup SSH access
eval "$(ssh-agent)"
ssh-add -D

# HACK: This is a fragile attempt to add whatever key is for github.com to our ssh-agent
grep -A 2 github.com ~/.ssh/config | grep IdentityFile | awk '{ print $2 }' | xargs -n 1 ssh-add
