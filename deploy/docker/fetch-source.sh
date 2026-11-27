#!/bin/sh
# Fetches one upstream source tree at a tag and verifies that the tag points
# to the expected commit, so a moved or replaced tag fails the build.
# Usage: fetch-source <git-url> <tag> <commit> <dir>
set -eu
url=$1 tag=$2 commit=$3 dir=$4

case "$commit" in
  "" | *[!0-9a-f]*)
    echo "fetch-source: invalid commit for $url: '$commit'" >&2
    exit 1
    ;;
esac
if [ "${#commit}" -ne 40 ] || [ "$commit" = "0000000000000000000000000000000000000000" ]; then
  echo "fetch-source: commit for $url is not pinned; run scripts/update-worker-pins.sh" >&2
  exit 1
fi

git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" "$url" "$dir"
actual=$(git -C "$dir" rev-parse HEAD)
if [ "$actual" != "$commit" ]; then
  echo "fetch-source: $url $tag is at $actual, expected $commit" >&2
  exit 1
fi
rm -rf "$dir/.git"
