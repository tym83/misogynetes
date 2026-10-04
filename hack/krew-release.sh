#!/usr/bin/env bash
# Builds the krew archives for one release and writes the krew manifest
# with their sha256 filled in.
#
#   hack/krew-release.sh v0.1.0 dist
#
# Produces dist/misogyn-<version>-<os>-<arch>.tar.gz for darwin and linux
# on amd64 and arm64, each holding kubectl-misogyn, LICENSE and README.md,
# and dist/misogyn.yaml, made from plugins/misogyn.yaml. Runs on macOS
# (bash 3.2) and Linux.
set -euo pipefail

plugin=misogyn
platforms="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"

version=${1:?usage: $0 <version, e.g. v0.1.0> <output dir>}
out=${2:?usage: $0 <version, e.g. v0.1.0> <output dir>}
case $version in
v[0-9]*.[0-9]*.[0-9]*) ;;
*) echo "$0: version must look like v1.2.3, got $version" >&2; exit 1 ;;
esac

root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

sums=""
for p in $platforms; do
	os=${p%/*}
	arch=${p#*/}
	dir="$stage/$os-$arch"
	mkdir -p "$dir"
	(cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
		go build -trimpath -ldflags="-s -w" -o "$dir/kubectl-$plugin" .)
	cp "$root/LICENSE" "$root/README.md" "$dir/"
	archive="$out/$plugin-$version-$os-$arch.tar.gz"
	COPYFILE_DISABLE=1 tar -czf "$archive" -C "$dir" "kubectl-$plugin" LICENSE README.md
	sums="$sums $os-$arch=$(sha256 "$archive")"
	echo "built $archive"
done

# Every version in the template becomes $version; the sha256 line after
# each uri gets the checksum of that uri's archive.
awk -v version="$version" -v sums="$sums" '
	BEGIN {
		n = split(sums, pairs, " ")
		for (i = 1; i <= n; i++) {
			split(pairs[i], kv, "=")
			sum[kv[1]] = kv[2]
		}
	}
	/^  version: / { sub(/v[0-9]+\.[0-9]+\.[0-9]+/, version) }
	/^ *uri: / {
		gsub(/v[0-9]+\.[0-9]+\.[0-9]+/, version)
		key = $0
		sub(/\.tar\.gz.*/, "", key)
		sub(/.*-/, "", key)
		os_key = $0
		sub(/-[a-z0-9]+\.tar\.gz.*/, "", os_key)
		sub(/.*-/, "", os_key)
		pending = os_key "-" key
	}
	/^ *sha256: / && pending != "" {
		if (!(pending in sum)) { print "no archive for " pending > "/dev/stderr"; exit 1 }
		sub(/".*"/, "\"" sum[pending] "\"")
		pending = ""
	}
	{ print }
' "$root/plugins/$plugin.yaml" >"$out/$plugin.yaml"
echo "wrote $out/$plugin.yaml"
