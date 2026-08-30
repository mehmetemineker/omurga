#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-0.1.0}"
ARCHES="${OMURGA_DEB_ARCHES:-amd64 arm64}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"

if [[ ! "${VERSION}" =~ ^[0-9][0-9A-Za-z.+:~-]*$ ]]; then
  echo "invalid Debian package version: ${VERSION}" >&2
  exit 1
fi

mkdir -p "${DIST_DIR}"

for arch in ${ARCHES}; do
  case "${arch}" in
    amd64) goarch=amd64 ;;
    arm64) goarch=arm64 ;;
    *) echo "unsupported Debian architecture: ${arch}" >&2; exit 1 ;;
  esac

  stage="$(mktemp -d)"
  trap 'rm -rf "${stage}"' EXIT
  chmod 0755 "${stage}"
  mkdir -p "${stage}/DEBIAN" "${stage}/usr/bin" "${stage}/usr/share/doc/omurga"

  sed -e "s/@VERSION@/${VERSION}/" -e "s/@ARCH@/${arch}/" \
    "${ROOT_DIR}/packaging/deb/control.template" > "${stage}/DEBIAN/control"

  GOOS=linux GOARCH="${goarch}" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X omurga/internal/buildinfo.Version=${VERSION} -X omurga/internal/buildinfo.Commit=${OMURGA_COMMIT:-unknown} -X omurga/internal/buildinfo.Date=${OMURGA_BUILD_DATE:-unknown}" \
    -o "${stage}/usr/bin/omurga" "${ROOT_DIR}/cmd/omurga"
  chmod 0755 "${stage}/usr/bin/omurga"
  install -m 0644 "${ROOT_DIR}/README.md" "${stage}/usr/share/doc/omurga/README.md"

  output="${DIST_DIR}/omurga_${VERSION}_${arch}.deb"
  rm -f "${output}"
  dpkg-deb --build --root-owner-group "${stage}" "${output}" >/dev/null
  echo "created ${output}"
done
