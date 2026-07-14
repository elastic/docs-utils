#!/usr/bin/env bash
# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Apache License,
# Version 2.0 (the "License"); you may not use this file except in compliance
# with the License. You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

# Creates and pushes a semver tag. The tag triggers the GitHub Actions release
# workflow, which builds the archives and uploads install.sh and install.ps1.
set -euo pipefail

usage() {
  echo "Usage: ./scripts/release.sh <major.minor.patch[-prerelease][+build]>" >&2
  exit 2
}

[[ $# -eq 1 ]] || usage
version="${1#v}"
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]] || usage

[[ "$(git branch --show-current)" == "main" ]] || {
  echo "Releases must be created from main." >&2
  exit 1
}
git diff --quiet && git diff --cached --quiet || {
  echo "Working tree must be clean before creating a release." >&2
  exit 1
}

tag="v${version}"
git rev-parse --verify --quiet "refs/tags/${tag}" >/dev/null && {
  echo "Tag ${tag} already exists locally." >&2
  exit 1
}
if git ls-remote --exit-code --tags origin "refs/tags/${tag}" >/dev/null 2>&1; then
  echo "Tag ${tag} already exists on origin." >&2
  exit 1
fi

git tag -a "$tag" -m "Release ${tag}"
git push origin "$tag"
