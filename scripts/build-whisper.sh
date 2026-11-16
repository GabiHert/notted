#!/usr/bin/env bash
# Builds the pinned whisper.cpp release as static libraries with Metal (its shaders embedded), for
# the cgo package internal/integration/speech/whispercpp. Output: helpers/whisper/build/install
# (include/ and lib/). Skips the work when that build is already there for this version.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=v1.8.6
SHA256=f8e632016ceae556f3132a16c7f704be1e7715595041f474fa81a2b64c1abf7c
URL="https://github.com/ggml-org/whisper.cpp/archive/refs/tags/$VERSION.tar.gz"

OUT="$PWD/helpers/whisper/build"
PREFIX="$OUT/install"
STAMP="$PREFIX/.version"
if [ -f "$STAMP" ] && [ "$(cat "$STAMP")" = "$VERSION" ] && [ -f "$PREFIX/lib/libwhisper.a" ]; then
  echo "whisper.cpp $VERSION already built in $PREFIX"
  exit 0
fi

CMAKE="${CMAKE:-$(command -v cmake || echo /opt/homebrew/bin/cmake)}"
[ -x "$CMAKE" ] || { echo "build-whisper: cmake not found (brew install cmake)" >&2; exit 1; }

mkdir -p "$OUT"
TARBALL="$OUT/whisper.cpp-$VERSION.tar.gz"
if [ ! -f "$TARBALL" ] || ! shasum -a 256 "$TARBALL" | grep -q "^$SHA256 "; then
  echo "downloading whisper.cpp $VERSION"
  curl -fsSL -o "$TARBALL.part" "$URL"
  mv "$TARBALL.part" "$TARBALL"
fi
got="$(shasum -a 256 "$TARBALL" | awk '{print $1}')"
[ "$got" = "$SHA256" ] || { echo "build-whisper: sha256 mismatch: $got" >&2; rm -f "$TARBALL"; exit 1; }

SRC="$OUT/src"
rm -rf "$SRC" "$OUT/cmake" "$PREFIX"
mkdir -p "$SRC"
tar xzf "$TARBALL" -C "$SRC" --strip-components 1

export MACOSX_DEPLOYMENT_TARGET=14.0
"$CMAKE" -S "$SRC" -B "$OUT/cmake" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DCMAKE_OSX_DEPLOYMENT_TARGET=14.0 \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DBUILD_SHARED_LIBS=OFF \
  -DGGML_METAL=ON \
  -DGGML_METAL_EMBED_LIBRARY=ON \
  -DGGML_NATIVE=OFF \
  -DGGML_CPU_ARM_ARCH=armv8.2-a+dotprod+fp16 \
  -DGGML_OPENMP=OFF \
  -DWHISPER_BUILD_EXAMPLES=OFF \
  -DWHISPER_BUILD_TESTS=OFF \
  -DWHISPER_BUILD_SERVER=OFF \
  -DWHISPER_SDL2=OFF \
  -DWHISPER_CURL=OFF >/dev/null
"$CMAKE" --build "$OUT/cmake" --config Release -j "$(sysctl -n hw.ncpu)" >/dev/null
"$CMAKE" --install "$OUT/cmake" >/dev/null
cp "$SRC/LICENSE" "$PREFIX/LICENSE"
# The build tree and sources are not needed once installed.
rm -rf "$OUT/cmake" "$SRC"
echo "$VERSION" >"$STAMP"
echo "built whisper.cpp $VERSION in $PREFIX"
ls "$PREFIX/lib"
