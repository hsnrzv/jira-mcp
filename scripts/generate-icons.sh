#!/usr/bin/env bash
# Regenerates the committed icon assets from assets/icon.svg.
# Only needed after editing the SVG; build-release.sh uses the committed outputs.
# Requires: Google Chrome (SVG rendering), python3 + Pillow, macOS iconutil.
set -euo pipefail
cd "$(dirname "$0")/.."
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

"$CHROME" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 \
  --window-size=1024,1024 --screenshot="$tmp/icon-1024.png" "file://$PWD/assets/icon.svg" 2>/dev/null

python3 - "$tmp" <<'PY'
import sys
from PIL import Image
tmp = sys.argv[1]
src = Image.open(f"{tmp}/icon-1024.png").convert("RGBA")
src.save("assets/icon.png")
iconset = f"{tmp}/AppIcon.iconset"
import os; os.makedirs(iconset)
for pt in (16, 32, 128, 256, 512):
    for scale in (1, 2):
        px = pt * scale
        name = f"icon_{pt}x{pt}{'@2x' if scale == 2 else ''}.png"
        src.resize((px, px), Image.LANCZOS).save(f"{iconset}/{name}")
# Windows/favicon: crop the tile (drop macOS padding) so it fills small sizes.
tile = src.crop((92, 92, 932, 932))
tile.save("assets/icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
tile.resize((64, 64), Image.LANCZOS).save("internal/setupui/favicon.png", optimize=True)
PY

iconutil -c icns -o assets/AppIcon.icns "$tmp/AppIcon.iconset"
ls -l assets internal/setupui/favicon.png
