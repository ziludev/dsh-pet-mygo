#!/bin/zsh
# Convert VP9-alpha .webm assets to HEVC-with-alpha .mov for WKWebView.
# Reuses dsh-pet's AVFoundation encoder (hevc_alpha_encoder.swift, MIT).
#   usage: ./scripts/encode-movs.sh [name ...]   (names without .webm; empty = all)
set -euo pipefail

WEBM_DIR=${DSH_PET_WEBM_DIR:-/Users/alexwang/.local/lib/node_modules/dsh-pet/assets/webm}
OUT_DIR=${DSH_PET_ANIM_DIR:-$(cd "$(dirname "$0")/.." && pwd)/resources/anim}
ENCODER_BIN=${DSH_PET_ENCODER:-/tmp/hevc-alpha-encoder}
FPS=24

# Build the encoder once if missing (AVFoundation + hevcWithAlpha, macOS only).
if [[ ! -x "$ENCODER_BIN" ]]; then
  SRC=${DSH_PET_ENCODER_SRC:-/tmp/hevc_alpha_encoder.swift}
  if [[ ! -f "$SRC" ]]; then
    print -u2 "encoder source not found: $SRC (set DSH_PET_ENCODER_SRC)"
    exit 1
  fi
  xcrun swiftc -O -framework AVFoundation -framework CoreVideo -framework VideoToolbox "$SRC" -o "$ENCODER_BIN"
fi

mkdir -p "$OUT_DIR"
names=("$@")
if (( ${#names} == 0 )); then
  for f in "$WEBM_DIR"/*.webm(N); do names+=("${f:t:r}"); done
fi

converted=0
for name in "${names[@]}"; do
  src="$WEBM_DIR/$name.webm"
  out="$OUT_DIR/$name.mov"
  [[ -f "$src" ]] || { print -u2 "missing: $src"; continue; }
  if [[ -f "$out" && "$out" -nt "$src" ]]; then
    print "skip (fresh): $name"; continue
  fi
  print "convert: $name"
  ffmpeg -hide_banner -loglevel error -c:v libvpx-vp9 -i "$src" -an \
    -f rawvideo -pix_fmt bgra pipe:1 \
    | "$ENCODER_BIN" "$out" 640 360 "$FPS" >/dev/null
  (( converted += 1 ))
done
print "done: $converted"
