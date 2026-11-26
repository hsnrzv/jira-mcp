#!/usr/bin/env bash
# Cross-compiles standalone binaries (no Docker, no runtime needed) into ./dist
# and wraps the macOS builds in a double-clickable .app bundle.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${VERSION:-dev}"
OUT=dist
rm -rf "$OUT"; mkdir -p "$OUT"

build() { # os arch outfile
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags="-s -w" -o "$3" ./cmd/jira-mcp
}

for arch in arm64 amd64; do
  app="$OUT/JiraMCP-macos-$arch/Jira MCP.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  build darwin "$arch" "$app/Contents/MacOS/jira-mcp"
  cp assets/AppIcon.icns "$app/Contents/Resources/AppIcon.icns"
  cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Jira MCP</string>
  <key>CFBundleIdentifier</key><string>io.github.hsnrzv.jira-mcp</string>
  <key>CFBundleExecutable</key><string>jira-mcp</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
</dict></plist>
PLIST
  (cd "$OUT" && zip -qr "JiraMCP-macos-$arch.zip" "JiraMCP-macos-$arch")
  if command -v hdiutil >/dev/null; then
    # Ad-hoc signing avoids the "damaged app" error on Apple Silicon.
    codesign --force --deep --sign - "$app" 2>/dev/null || true
    stage="$OUT/dmg-$arch"; mkdir -p "$stage"
    cp -R "$app" "$stage/"; ln -s /Applications "$stage/Applications"
    hdiutil create -quiet -volname "Jira MCP" -srcfolder "$stage" -ov -format UDZO "$OUT/JiraMCP-macos-$arch.dmg"
    rm -rf "$stage"
  fi
done

# Embed the icon into the Windows exe via a .syso resource (picked up by go build).
syso=cmd/jira-mcp/rsrc_windows_amd64.syso
trap 'rm -f "$syso"' EXIT
go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out cmd/jira-mcp/rsrc \
  --icon assets/icon.ico --manifest cli --product-name "Jira MCP" --file-description "Jira MCP" \
  || echo "warning: go-winres failed; Windows exe will have no icon" >&2
build windows amd64 "$OUT/JiraMCP-windows-amd64/JiraMCP.exe"
rm -f "$syso"
(cd "$OUT" && zip -qr JiraMCP-windows-amd64.zip JiraMCP-windows-amd64)
build linux amd64 "$OUT/JiraMCP-linux-amd64/jira-mcp"
ls -1 "$OUT"/*.zip "$OUT"/*.dmg
