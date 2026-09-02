#!/usr/bin/env bash
set -eu

REPOSITORY="mehmetemineker/omurga"
INSTALL_ROOT="https://github.com/${REPOSITORY}"

fail() {
  echo "omurga installer: $*" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v dpkg >/dev/null 2>&1 || fail "dpkg is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"

if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
else
  command -v sudo >/dev/null 2>&1 || fail "sudo is required when the installer is not run as root"
  SUDO="sudo"
fi

if [ -r /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
else
  fail "/etc/os-release was not found"
fi

case "${ID:-}" in
  debian|ubuntu) ;;
  *) fail "unsupported distribution: ${ID:-unknown}; Debian and Ubuntu are supported" ;;
esac

architecture="$(dpkg --print-architecture)"
case "$architecture" in
  arm64|amd64) ;;
  *) fail "unsupported architecture: $architecture; arm64 and amd64 are supported" ;;
esac

latest_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "${INSTALL_ROOT}/releases/latest")"
latest_url="${latest_url%/}"
tag="${latest_url##*/}"
case "$tag" in
  v[0-9]*) ;;
  *) fail "could not determine the latest release tag" ;;
esac
version="${tag#v}"
package="omurga_${version}_${architecture}.deb"
download_url="${INSTALL_ROOT}/releases/download/${tag}/${package}"
checksums_url="${INSTALL_ROOT}/releases/download/${tag}/SHA256SUMS"

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT

package_path="${temporary_directory}/${package}"
checksums_path="${temporary_directory}/SHA256SUMS"

echo "Downloading Omurga ${tag} for ${architecture}..."
curl -fL --retry 3 -o "$package_path" "$download_url"
curl -fL --retry 3 -o "$checksums_path" "$checksums_url"

expected_checksum="$(awk -v file="dist/${package}" '$2 == file { print $1; exit }' "$checksums_path")"
[ -n "$expected_checksum" ] || fail "the release checksum for ${package} was not found"
actual_checksum="$(sha256sum "$package_path" | awk '{print $1}')"
[ "$actual_checksum" = "$expected_checksum" ] || fail "checksum verification failed for ${package}"

echo "Installing ${package}..."
$SUDO apt-get install -y "$package_path"

echo "Installed:"
omurga version
