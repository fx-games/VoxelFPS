#!/usr/bin/env bash
# Build the browser version and publish it to https://ruzzoli.de/games/janus-station/play/
set -euo pipefail
cd "$(dirname "$0")"
./build_web.sh
rsync -az --delete /Users/felix/Projects/fx-games/site/janus-station/play/ ruzzoli.de:/var/www/ruzzoli.de/games/janus-station/play/
echo "published: https://ruzzoli.de/games/janus-station/play/"
