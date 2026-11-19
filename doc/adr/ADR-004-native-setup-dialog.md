# ADR-004: Native setup dialog without CGO

**Status:** Accepted

## Context
Double-clicking the app opened a localhost web form in the browser. Users expect a native window, but release builds are cross-compiled with `CGO_ENABLED=0` from one machine (macOS `.app`, Windows `.exe`, Linux).

## Decision
`jira-mcp setup` shows one native window for the whole wizard by running a small embedded script with a toolkit that ships with the OS:
- macOS: `osascript -l JavaScript` builds a regular Cocoa `NSWindow` (title bar with close/minimize, Dock icon, normal window level; text fields, secure PAT field, search field, scrollable checkbox list, checkbox).
- Windows: Windows PowerShell 5.1 builds a WinForms window (`-NoProfile`, `-STA`; minimize/close buttons, taskbar entry, not top-most; `CheckedListBox` with a filter box for projects). A tiny `-EncodedCommand` bootstrap reads the script from the first stdin line, so the command line stays below the 32 767-character limit.

The window process lives for the whole wizard. Go sends JSON-line commands on its stdin (`init`, `connect`, `projects`, `register`, `status`, `copy`, `quit`) and reads JSON-line user actions from its stdout (`next`, `back`, `save`, `choose`, `close`). While Go contacts Jira the window stays open, disables its buttons and shows a spinner; errors appear inline in a red box on the current page (typed values are kept).

The wizard has two steps: *Connect* (URL, PAT, CA file → *Next*, which signs in and lists projects, so it doubles as the connection test) and *Projects* (multi-select of every project visible to the PAT, write toggle → *Back* / *Save*), then a *Register* page (install/copy buttons). Every page ends with a red *Close server* button; it, the window close button and ⌘Q end the setup process and abort an in-flight Jira call. Asking Jira for the project list removes typos in `JIRA_ALLOWED_PROJECTS`. The wizard logic (`internal/nativeui.Flow`) shares validation, normalization, project listing and install code with the web UI (`setupui.NormalizeConnection`, `Normalize`, `Projects`, `Validate`, `Targets`); the browser form follows the same two steps (`POST /api/projects`). Linux, failures to start the dialog, and `setup --web` use the existing browser form.

## Consequences
+ Native look, no browser tab, no local HTTP listener in the default path; still a single static binary.
+ One window that never closes/re-opens between steps, live progress, inline errors; it can be minimized and does not block the desktop.
+ Flow is unit-tested with a fake `Window`; the pipe transport is unit-tested; the macOS script has an integration test that drives the real window.
− The Windows script cannot be exercised in CI on macOS/Linux (only its protocol strings and the bootstrap are tested).
− Locked-down Windows (Constrained Language Mode) falls back to the browser form.

## Alternatives Rejected
- WebView/Wails/Fyne/Gio: need CGO or per-OS toolchains, break cross-compilation, and add large dependencies.
- Sequential `display dialog` prompts: one field per dialog, no checkbox, poor UX.
- One modal `NSAlert`/WinForms dialog per step (previous design): the dialog closed and re-opened on every button, errors were easy to miss, and the floating modal had no minimize/close buttons and blocked the desktop.
