// JavaScript for Automation (osascript -l JavaScript). Inputs arrive only via
// JMCP_* environment variables; the result is the return value of run().
ObjC.import('Cocoa');

function env(name) {
  const v = $.NSProcessInfo.processInfo.environment.objectForKey(name);
  return v.isNil() ? '' : ObjC.unwrap(v);
}

function setupApp() {
  const app = $.NSApplication.sharedApplication;
  app.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
  // Without an Edit menu, Cmd+X/C/V/A do not reach the text fields.
  const main = $.NSMenu.alloc.init;
  const item = $.NSMenuItem.alloc.init;
  main.addItem(item);
  const edit = $.NSMenu.alloc.initWithTitle('Edit');
  [['Undo', 'undo:', 'z'], ['Redo', 'redo:', 'Z'], ['Cut', 'cut:', 'x'], ['Copy', 'copy:', 'c'],
   ['Paste', 'paste:', 'v'], ['Select All', 'selectAll:', 'a']]
    .forEach(([t, a, k]) => edit.addItemWithTitleActionKeyEquivalent(t, a, k));
  item.submenu = edit;
  app.mainMenu = main;
  return app;
}

function newAlert() {
  const alert = $.NSAlert.alloc.init;
  alert.messageText = env('JMCP_TITLE');
  alert.informativeText = env('JMCP_MESSAGE');
  const icon = env('JMCP_ICON_PNG');
  if (icon) {
    const data = $.NSData.alloc.initWithBase64EncodedStringOptions(icon, 0);
    if (!data.isNil()) {
      const img = $.NSImage.alloc.initWithData(data);
      if (!img.isNil()) alert.icon = img;
    }
  }
  return alert;
}

// Cocoa constants are bridged as strings, so compare button codes as numbers.
const FIRST_BUTTON = 1000;
const ON = 1;

function runAlert(app, alert) {
  alert.layout;
  // Stay visible even if macOS refuses to activate this helper process.
  alert.window.level = $.NSFloatingWindowLevel;
  app.activateIgnoringOtherApps(true);
  return Number(alert.runModal);
}
