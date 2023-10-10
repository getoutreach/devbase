#!/usr/bin/env bash
#
# Original License:
#
# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Based off of: https://github.com/kubernetes/enhancements/blob/master/hack/update-toc.sh
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Version of the toc generator to use
TOOL_VERSION=b8c54a57d69f29386d055584e595f38d65ce2a1f

mdtoc=$("$DIR/shell-wrapper.sh" gobin.sh -p "sigs.k8s.io/mdtoc@$TOOL_VERSION")

# Update tables of contents if necessary.
find "$DIR/../rfcs" -name '*.md' -exec "$mdtoc" --inplace --max-depth=5 {} +
