# ADR-004: Native setup dialog without CGO

**Status:** Accepted

## Context
Double-clicking the app opened a localhost web form in the browser. Users expect a native window, but release builds are cross-compiled with `CGO_ENABLED=0` from one machine (macOS `.app`, Windows `.exe`, Linux).

## Decision
`jira-mcp setup` shows native modal dialogs by running small embedded scripts with toolkits that ship with the OS:
- macOS: `osascript -l JavaScript` builds a Cocoa `NSAlert` with an accessory form (text fields, secure PAT field, checkbox).
- Windows: Windows PowerShell 5.1 builds a WinForms dialog (`-EncodedCommand`, `-NoProfile`, `-STA`).

Inputs are passed via `JMCP_*` environment variables and results are read as JSON from stdout. The wizard logic (`internal/nativeui.Flow`) shares validation, normalization and install code with the web UI (`setupui.Normalize`, `Validate`, `Targets`). Linux, failures to start the dialog, and `setup --web` use the existing browser form.

## Consequences
+ Native look, no browser tab, no local HTTP listener in the default path; still a single static binary.
+ Flow is unit-tested with a fake `Dialogs`; the macOS backend has an integration test.
− The dialog closes briefly while Jira is contacted; no live progress indicator.
− Locked-down Windows (Constrained Language Mode) falls back to the browser form.

## Alternatives Rejected
- WebView/Wails/Fyne/Gio: need CGO or per-OS toolchains, break cross-compilation, and add large dependencies.
- Sequential `display dialog` prompts: one field per dialog, no checkbox, poor UX.
