# Runbook: Desktop app (no Docker, no clone)

## For colleagues
1. Download for your OS (macOS Apple Silicon: `JiraMCP-macos-arm64.dmg`, drag *Jira MCP* to Applications; others: zip, unzip).
   - macOS: `Jira MCP.app` (first run: right-click → Open, because the app is not notarised).
   - Windows: `JiraMCP.exe` (SmartScreen → More info → Run anyway). A console window stays open while setup runs.
2. Double-click. A native setup window opens (macOS: Cocoa window with a Dock icon; Windows: WinForms window with a taskbar entry). It has the usual minimize and close buttons; closing it stops setup. Linux, or any machine where the native dialog cannot start, falls back to a local browser form (`http://127.0.0.1:<port>`).
3. **Step 1 – Connect:** paste **Jira URL** and **PAT** (optional: corporate root CA file) and click *Next*. The app signs in (`/rest/api/2/myself`) and loads every project the PAT can see (`/rest/api/2/project`); a spinner shows progress. If this fails (wrong URL/PAT, proxy, missing permission), the window stays on step 1 and shows the error in a red box; fix the values and click *Next* again.
4. **Step 2 – Projects:** tick the project(s) the MCP server may access (`JIRA_ALLOWED_PROJECTS`; type in the filter box to narrow long lists; previously saved projects are pre-ticked), optionally tick *Allow write tools*, then click *Save* (no separate connection test is needed: reaching step 2 proves the connection works). *Back* returns to step 1.
5. On the next page click *Install in GitHub Copilot (IntelliJ)* (`~/.config/github-copilot/intellij/mcp.json`) and/or *Install in Copilot CLI / Air* (`~/.copilot/mcp-config.json`): the `jira` entry is merged in, other servers are kept and a `.bak` backup is written (files containing comments are refused; use the manual snippet). Restart the IDE. Alternatively click *Copy VS Code config* / *Copy JetBrains / Claude config* and paste the snippet into your AI tool's MCP config (VS Code/Copilot `mcp.json`, JetBrains, Claude Desktop). The snippet points at the app binary with the `serve` argument; the IDE starts it on demand.
6. Click the red *Close server* button (always the last button; same as closing the window or ⌘Q) to stop the setup app. Re-run the app any time to change settings.

Write tools are off unless *Allow write tools* is ticked. The selected projects restrict all access to those projects.

## Modes
| Command | Behaviour |
|---|---|
| `jira-mcp` / `jira-mcp setup` | Opens the native setup window (default for double-click); falls back to the browser form |
| `jira-mcp setup --web` | Forces the browser form |
| `jira-mcp serve` | MCP over stdio (what the IDE runs) |

Settings live in `<user config dir>/jira-mcp/settings.json` (0600): `~/Library/Application Support` (macOS), `%AppData%` (Windows), `~/.config` (Linux). Environment variables (see README) override saved values. Keep the binary at a stable path — the IDE config references it.

## Native window implementation
`internal/nativeui` stays CGO-free so releases still cross-compile: it runs one embedded script per setup run with OS-provided toolkits — `/usr/bin/osascript -l JavaScript` (Cocoa `NSWindow`) on macOS and `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe` (WinForms) on Windows. The window process stays open for the whole wizard: Go sends JSON-line commands (page contents, status, clipboard text) on its stdin and reads JSON-line user actions from its stdout. The PAT is never sent to the window; it only comes back on stdout when the user types one. On Windows a short `-EncodedCommand` bootstrap reads the script itself from the first stdin line. If PowerShell is blocked (e.g. Constrained Language Mode/AppLocker) the app falls back to the browser form automatically. See ADR-004.

## Building releases
`scripts/build-release.sh` (needs Go + `zip`) cross-compiles static binaries into `dist/` (macOS arm64/amd64 `.app` bundles, Windows exe, Linux binary).

## App icon
`assets/icon.svg` is the source (original design: split diamond = tilted-tile/`< >` motif, MCP node hub, AI sparkles). Committed derivatives: `assets/AppIcon.icns` (copied into `Contents/Resources` of the macOS bundle, `CFBundleIconFile`), `assets/icon.ico` (embedded into the Windows exe via a temporary `.syso` generated with `go run github.com/tc-hib/go-winres`), `assets/icon.png`, and `internal/setupui/favicon.png` (setup UI). After editing the SVG run `scripts/generate-icons.sh` (needs Chrome, Python + Pillow, macOS `iconutil`).

If an older icon-less copy is still shown in Finder/Launchpad after reinstalling, refresh the icon cache: `touch "/Applications/Jira MCP.app" && killall Dock`.
