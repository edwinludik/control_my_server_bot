#!/usr/bin/env bash
# Prints the version CI should release next (without the "v" prefix).
#
#   - If the version in the VERSION file has no tag yet, it is released as-is.
#     This lets a maintainer pick a version by hand (e.g. a major bump).
#   - Otherwise the minor version is incremented and the patch reset to 0,
#     starting from the highest of VERSION and all existing vX.Y.Z tags.
#
# Run from the repository root with all tags fetched.
set -euo pipefail

current=$(tr -d '[:space:]' < VERSION)

if ! [[ "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "VERSION must look like MAJOR.MINOR.PATCH, got: '$current'" >&2
	exit 1
fi

if ! git rev-parse -q --verify "refs/tags/v${current}" >/dev/null; then
	echo "$current"
	exit 0
fi

highest=$(git tag -l 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sed 's/^v//' | sort -V | tail -n 1 || true)
base=$(printf '%s\n%s\n' "$current" "$highest" | sed '/^$/d' | sort -V | tail -n 1)

IFS=. read -r major minor _ <<<"$base"
echo "${major}.$((minor + 1)).0"
