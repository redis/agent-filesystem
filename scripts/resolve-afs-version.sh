#!/bin/sh
set -eu

# Clean product versioning for the AFS CLI/control plane.
#
# Accepted release tags:
#   vX.Y.Z
#   afs-vX.Y.Z
#
# SDK/package tags such as redis-afs-python-vX.Y.Z are intentionally ignored.

base_version="${AFS_VERSION_BASE:-v0.1.0}"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	printf '%s\n' "${base_version}-dev"
	exit 0
fi

short_sha="$(git rev-parse --short=7 HEAD 2>/dev/null || true)"
if [ -z "$short_sha" ]; then
	printf '%s\n' "${base_version}-dev"
	exit 0
fi

dirty=0
if ! git diff --quiet --ignore-submodules -- 2>/dev/null; then
	dirty=1
fi
if ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
	dirty=1
fi

tag="$(
	git describe \
		--tags \
		--abbrev=0 \
		--match 'v[0-9]*' \
		--match 'afs-v[0-9]*' \
		2>/dev/null || true
)"

if [ -n "$tag" ]; then
	clean_tag="${tag#afs-}"
	distance="$(git rev-list --count "${tag}..HEAD" 2>/dev/null || echo 0)"
	if [ "$distance" = "0" ]; then
		version="$clean_tag"
	else
		version="${clean_tag}-dev.${distance}+g${short_sha}"
	fi
else
	version="${base_version}-dev+g${short_sha}"
fi

if [ "$dirty" = "1" ]; then
	case "$version" in
		*+*) version="${version}.dirty" ;;
		*) version="${version}+dirty" ;;
	esac
fi

printf '%s\n' "$version"
