# Runbook: Desktop app (no Docker, no clone)

## For colleagues
1. Download for your OS (macOS Apple Silicon: `JiraMCP-macos-arm64.dmg`, drag *Jira MCP* to Applications; others: zip, unzip).
   - macOS: `Jira MCP.app` (first run: right-click → Open, because the app is not notarised).
   - Windows: `JiraMCP.exe` (SmartScreen → More info → Run anyway). A console window stays open while setup runs.
2. Double-click. A native setup dialog opens (macOS: Cocoa alert; Windows: WinForms window). Linux, or any machine where the native dialog cannot start, falls back to a local browser form (`http://127.0.0.1:<port>`).
3. Paste **Jira URL**, **PAT**, **project key(s)**; click *Test connection*, then *Save*. The dialog briefly closes while Jira is contacted and re-opens with the result.
4. In the follow-up dialog click *Install in GitHub Copilot (IntelliJ)* (`~/.config/github-copilot/intellij/mcp.json`) and/or *Install in Copilot CLI / Air* (`~/.copilot/mcp-config.json`): the `jira` entry is merged in, other servers are kept and a `.bak` backup is written (files containing comments are refused; use the manual snippet). Restart the IDE. Alternatively click *Copy VS Code config* / *Copy JetBrains / Claude config* and paste the snippet into your AI tool's MCP config (VS Code/Copilot `mcp.json`, JetBrains, Claude Desktop). The snippet points at the app binary with the `serve` argument; the IDE starts it on demand.
5. Click *Done* (browser form: *Close*). Re-run the app any time to change settings.

Write tools are off unless *Allow write tools* is ticked. Project keys restrict all access to those projects.

## Modes
| Command | Behaviour |
|---|---|
| `jira-mcp` / `jira-mcp setup` | Opens the native setup dialog (default for double-click); falls back to the browser form |
| `jira-mcp setup --web` | Forces the browser form |
| `jira-mcp serve` | MCP over stdio (what the IDE runs) |

Settings live in `<user config dir>/jira-mcp/settings.json` (0600): `~/Library/Application Support` (macOS), `%AppData%` (Windows), `~/.config` (Linux). Environment variables (see README) override saved values. Keep the binary at a stable path — the IDE config references it.

## Native dialog implementation
`internal/nativeui` stays CGO-free so releases still cross-compile: it runs embedded scripts with OS-provided toolkits — `/usr/bin/osascript -l JavaScript` (Cocoa `NSAlert`) on macOS and `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe` (WinForms) on Windows. Prefill values go in via `JMCP_*` environment variables (never the PAT); results come back on the child's stdout. If PowerShell is blocked (e.g. Constrained Language Mode/AppLocker) the app falls back to the browser form automatically. See ADR-004.

## Building releases
`scripts/build-release.sh` (needs Go + `zip`) cross-compiles static binaries into `dist/` (macOS arm64/amd64 `.app` bundles, Windows exe, Linux binary).

## App icon
`assets/icon.svg` is the source (original design: split diamond = tilted-tile/`< >` motif, MCP node hub, AI sparkles). Committed derivatives: `assets/AppIcon.icns` (copied into `Contents/Resources` of the macOS bundle, `CFBundleIconFile`), `assets/icon.ico` (embedded into the Windows exe via a temporary `.syso` generated with `go run github.com/tc-hib/go-winres`), `assets/icon.png`, and `internal/setupui/favicon.png` (setup UI). After editing the SVG run `scripts/generate-icons.sh` (needs Chrome, Python + Pillow, macOS `iconutil`).

If an older icon-less copy is still shown in Finder/Launchpad after reinstalling, refresh the icon cache: `touch "/Applications/Jira MCP.app" && killall Dock`.
