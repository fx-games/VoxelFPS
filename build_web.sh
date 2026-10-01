#!/usr/bin/env bash
# Build the browser version (WebAssembly + WebGL2) into the site's play/ folder.
# The page itself, play/index.html, lives in the site and is not touched here.
set -euo pipefail
cd "$(dirname "$0")"
OUT="${OUT:-/Users/felix/Projects/fx-games/site/janus-station/play}"
mkdir -p "$OUT"
# Files raymap only stats, mounted by size without their bytes (see web_js.go).
{ find assets/sprites/actors -type f ! -name '*.json' ! -path '*/humans/scientist/*' ! -name '.*'
  find assets/weapons/_sprites -type f ! -name '.*'; } | sort | while read -r f; do
  echo "$(stat -f %z "$f" 2>/dev/null || stat -c %s "$f") $f"; done > web_stubs.txt
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o "$OUT/janus.wasm" .
gzip -9 -k -f "$OUT/janus.wasm"   # nginx gzip_static serves this one
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT/"
ls -l "$OUT"/janus.wasm*
