#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?usage: package-release.sh VERSION OUTPUT_DIR [OS/ARCH ...]}"
DIST="${2:?output directory required}"
shift 2
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "Invalid version" >&2; exit 1; }
[[ ! -e "$DIST" ]] || { echo "Output directory must not exist" >&2; exit 1; }
VERSION_NUMBER="${VERSION#v}"

mkdir -p "$DIST"

targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

if (( $# )); then targets=("$@"); fi

for target in "${targets[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"

  NAME="graybox_${VERSION_NUMBER}_${GOOS}_${GOARCH}"
  WORK="${DIST}/${NAME}"

  mkdir -p "$WORK"

  BINARY="graybox"
  if [[ "$GOOS" == "windows" ]]; then
    BINARY="graybox.exe"
  fi

  echo "Building ${GOOS}/${GOARCH}"

  CGO_ENABLED=0 \
  GOOS="$GOOS" \
  GOARCH="$GOARCH" \
  go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o "${WORK}/${BINARY}" \
    ./cmd/graybox

  if [[ "$GOOS" == "windows" ]]; then
    python3 - "$WORK/$BINARY" "$DIST/$NAME.zip" <<'PYZIP'
import pathlib
import sys
import zipfile
binary = pathlib.Path(sys.argv[1])
with zipfile.ZipFile(sys.argv[2], 'w', zipfile.ZIP_DEFLATED) as archive:
    archive.write(binary, binary.name)
PYZIP
  else
    tar \
      -C "$WORK" \
      -czf "${DIST}/${NAME}.tar.gz" \
      "$BINARY"
  fi

  rm -rf "$WORK"
done
python3 scripts/verify-release.py --write-checksums "$DIST"
