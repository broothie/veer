#!/bin/sh

set -eu

REPO="${REPO:-broothie/veer}"
VERSION="${VERSION:-latest}"
BIN_DIR="${BIN_DIR:-}"

say() {
	printf '%s\n' "$*"
}

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

detect_os() {
	case "$(uname -s)" in
		Linux) printf 'linux\n' ;;
		Darwin) printf 'darwin\n' ;;
		*) fail "unsupported OS: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64) printf 'amd64\n' ;;
		aarch64|arm64) printf 'arm64\n' ;;
		*) fail "unsupported architecture: $(uname -m)" ;;
	esac
}

choose_bin_dir() {
	if [ -n "$BIN_DIR" ]; then
		printf '%s\n' "$BIN_DIR"
		return
	fi

	if [ "$(id -u)" -eq 0 ]; then
		printf '/usr/local/bin\n'
		return
	fi

	if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		printf '/usr/local/bin\n'
		return
	fi

	printf '%s/.local/bin\n' "$HOME"
}

download_url() {
	os="$1"
	arch="$2"
	filename="veer_${version_number}_$(printf '%s' "$os")_$(printf '%s' "$arch").tar.gz"

	if [ "$VERSION" = "latest" ]; then
		printf 'https://github.com/%s/releases/latest/download/%s\n' "$REPO" "$filename"
		return
	fi

	printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$VERSION" "$filename"
}

need_cmd curl
need_cmd tar
need_cmd install

os="$(detect_os)"
arch="$(detect_arch)"
bin_dir="$(choose_bin_dir)"
version_number="${VERSION#v}"

if [ "$VERSION" = "latest" ]; then
	version_number="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's#.*/tag/v\{0,1\}##')"
fi

[ -n "$version_number" ] || fail "could not determine release version"

url="$(download_url "$os" "$arch")"
tmpdir="$(mktemp -d)"
archive="$tmpdir/veer.tar.gz"

cleanup() {
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

say "Installing veer ${version_number} for ${os}/${arch}"
say "Download: $url"
say "Target: $bin_dir/veer"

mkdir -p "$bin_dir"
curl -fsSL -o "$archive" "$url"
tar -xzf "$archive" -C "$tmpdir"

if [ -w "$bin_dir" ] || [ "$(id -u)" -eq 0 ]; then
	install "$tmpdir/veer" "$bin_dir/veer"
else
	sudo install "$tmpdir/veer" "$bin_dir/veer"
fi

say "Installed veer to $bin_dir/veer"

case ":$PATH:" in
	*:"$bin_dir":*) ;;
	*)
		if [ "$bin_dir" = "$HOME/.local/bin" ]; then
			say "Add $bin_dir to PATH if it is not already available in your shell."
		fi
		;;
esac
