#!/bin/sh
# Copyright 2026 The EnvDoctor Authors
# SPDX-License-Identifier: Apache-2.0
#
# envdoctor installer — usage:
#
#   curl -fsSL https://reswaraa.github.io/envdoctor/install.sh | sh
#
# What it does:
#   1. Detects OS (darwin|linux) and arch (amd64|arm64).
#   2. Resolves the release tag (latest by default, or $ENVDOCTOR_VERSION).
#   3. Downloads the matching tarball + sha256sums.txt from GitHub Releases.
#   4. Verifies the tarball SHA-256 against sha256sums.txt.
#   5. Installs the binary to one of (first writable + on $PATH wins):
#        ~/.local/bin/envdoctor
#        /usr/local/bin/envdoctor
#      Never auto-sudo; if neither location is writable, the script
#      prints copy-pasteable instructions and exits non-zero.
#
# Environment overrides:
#   ENVDOCTOR_VERSION=v0.1.0       pin a specific tag (default: latest)
#   ENVDOCTOR_INSTALL_DIR=...      override install destination
#   ENVDOCTOR_REPO=owner/name      release source (default: reswaraa/envdoctor)
#   ENVDOCTOR_BASE_URL=https://... override the release host (testing)

set -eu

ENVDOCTOR_REPO="${ENVDOCTOR_REPO:-reswaraa/envdoctor}"
ENVDOCTOR_BASE_URL="${ENVDOCTOR_BASE_URL:-https://github.com}"

# ---- platform detection -----------------------------------------------------

detect_os() {
	case "$(uname -s)" in
		Darwin) echo darwin ;;
		Linux)  echo linux ;;
		*)
			printf 'envdoctor: unsupported OS %s (darwin and linux only in v0.x)\n' "$(uname -s)" >&2
			exit 1
			;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64) echo amd64 ;;
		arm64|aarch64) echo arm64 ;;
		*)
			printf 'envdoctor: unsupported arch %s (amd64 and arm64 only in v0.x)\n' "$(uname -m)" >&2
			exit 1
			;;
	esac
}

# ---- HTTP -------------------------------------------------------------------

# fetch URL to file (-o) or stdout. Prefers curl, falls back to wget.
fetch() {
	url=$1
	out=$2
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$out"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$url" -O "$out"
	else
		echo 'envdoctor: need curl or wget on PATH' >&2
		exit 1
	fi
}

# resolve_tag → echoes the tag to install. Uses the GitHub /releases/latest
# redirect: curl -fsSLI returns headers; we read the trailing Location.
resolve_tag() {
	if [ -n "${ENVDOCTOR_VERSION:-}" ]; then
		echo "$ENVDOCTOR_VERSION"
		return
	fi
	api_url="${ENVDOCTOR_BASE_URL}/${ENVDOCTOR_REPO}/releases/latest"
	# The latest-release endpoint 302s to /releases/tag/<tag>. We follow with
	# -L disabled and read the Location header so this works without jq.
	if command -v curl >/dev/null 2>&1; then
		location=$(curl -fsSI "$api_url" 2>/dev/null | awk 'tolower($1)=="location:" {print $2}' | tr -d '\r' | tail -1)
	else
		# wget --max-redirect=0 prints the Location to stderr; capture it.
		location=$(wget --max-redirect=0 -S "$api_url" 2>&1 | awk '/^  Location:/ {print $2}' | tail -1)
	fi
	if [ -z "$location" ]; then
		echo 'envdoctor: could not resolve latest release tag — set ENVDOCTOR_VERSION=vX.Y.Z to pin one' >&2
		exit 1
	fi
	# Strip everything up to and including /tag/.
	echo "$location" | sed 's#.*/tag/##'
}

# ---- checksum ---------------------------------------------------------------

# sha256_for_file picks the right hasher for the platform.
sha256_for_file() {
	file=$1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
	else
		echo 'envdoctor: need sha256sum or shasum on PATH' >&2
		exit 1
	fi
}

verify_sha256() {
	tarball=$1
	sumfile=$2
	expected_name=$(basename "$tarball")
	expected=$(awk -v f="$expected_name" '$2==f || $2=="*"f {print $1; exit}' "$sumfile")
	if [ -z "$expected" ]; then
		printf 'envdoctor: %s not listed in sha256sums.txt\n' "$expected_name" >&2
		exit 1
	fi
	actual=$(sha256_for_file "$tarball")
	if [ "$expected" != "$actual" ]; then
		printf 'envdoctor: checksum mismatch for %s\n  expected: %s\n  actual:   %s\n' \
			"$expected_name" "$expected" "$actual" >&2
		exit 1
	fi
}

# ---- install destination ----------------------------------------------------

# pick_install_dir echoes a writable directory on $PATH. Order:
#   1. $ENVDOCTOR_INSTALL_DIR (if set; created if missing)
#   2. ~/.local/bin           (created if missing, must be on PATH after)
#   3. /usr/local/bin         (must already exist + be writable)
# Refuses to auto-sudo. If nothing works, prints guidance and exits.
pick_install_dir() {
	if [ -n "${ENVDOCTOR_INSTALL_DIR:-}" ]; then
		mkdir -p "$ENVDOCTOR_INSTALL_DIR"
		echo "$ENVDOCTOR_INSTALL_DIR"
		return
	fi
	candidate="$HOME/.local/bin"
	mkdir -p "$candidate" 2>/dev/null || true
	if [ -w "$candidate" ]; then
		echo "$candidate"
		return
	fi
	candidate="/usr/local/bin"
	if [ -d "$candidate" ] && [ -w "$candidate" ]; then
		echo "$candidate"
		return
	fi
	cat >&2 <<EOF
envdoctor: no writable install directory found.

  Try one of:
    ENVDOCTOR_INSTALL_DIR="\$HOME/bin" curl -fsSL <url> | sh
    sudo mv envdoctor /usr/local/bin/
EOF
	exit 1
}

# ---- main -------------------------------------------------------------------

main() {
	os=$(detect_os)
	arch=$(detect_arch)
	tag=$(resolve_tag)
	version=${tag#v}

	# GoReleaser emits archives named envdoctor_<version>_<os>_<arch>.tar.gz
	# and a sha256sums.txt covering all artifacts at the release root.
	tarball_name="envdoctor_${version}_${os}_${arch}.tar.gz"
	tarball_url="${ENVDOCTOR_BASE_URL}/${ENVDOCTOR_REPO}/releases/download/${tag}/${tarball_name}"
	sums_url="${ENVDOCTOR_BASE_URL}/${ENVDOCTOR_REPO}/releases/download/${tag}/sha256sums.txt"

	tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/envdoctor.XXXXXX")
	# Trap so the temp dir is cleaned even on early exit.
	trap 'rm -rf "$tmpdir"' EXIT INT TERM

	printf 'envdoctor: downloading %s\n' "$tarball_name" >&2
	fetch "$tarball_url" "$tmpdir/$tarball_name"
	fetch "$sums_url" "$tmpdir/sha256sums.txt"

	printf 'envdoctor: verifying SHA-256\n' >&2
	verify_sha256 "$tmpdir/$tarball_name" "$tmpdir/sha256sums.txt"

	# tar -C extract; the archive contains a top-level envdoctor binary
	# alongside LICENSE / README / CHANGELOG. We only need the binary.
	tar -C "$tmpdir" -xzf "$tmpdir/$tarball_name"

	install_dir=$(pick_install_dir)
	dest="$install_dir/envdoctor"
	# Atomic install: write to a sibling tempfile, then mv. Avoids a partial
	# binary if the disk fills mid-copy.
	tmpdest="$dest.tmp.$$"
	cp "$tmpdir/envdoctor" "$tmpdest"
	chmod 0755 "$tmpdest"
	mv "$tmpdest" "$dest"

	printf 'envdoctor: installed %s → %s\n' "$tag" "$dest" >&2

	# PATH hint: if install_dir isn't on PATH, the user just won't find the
	# binary. The most common case is ~/.local/bin missing from shell rc.
	case ":$PATH:" in
		*":$install_dir:"*) ;;
		*)
			# shellcheck disable=SC2016
			# The literal $PATH in the export line is what we want printed.
			printf 'envdoctor: %s is NOT on $PATH — add to your shell rc:\n  export PATH="%s:$PATH"\n' \
				"$install_dir" "$install_dir" >&2
			;;
	esac

	# Sanity-print version so the user sees the install actually works.
	"$dest" version || true
}

main "$@"
