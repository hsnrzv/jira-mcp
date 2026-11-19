// JavaScript for Automation (osascript -l JavaScript). One persistent Cocoa
// window for the whole setup wizard. Commands arrive as JSON lines on stdin;
// user actions are written as JSON lines to stdout. The window has the usual
// close/minimize buttons; closing it (or "Close server", Cmd+Q) emits
// {"action":"close"} and exits.
ObjC.import('Cocoa');

const W = 460;         // content width
const M = 20;          // window margin
const LIST_H = 220, ROW = 22;

const ui = {
  win: null, host: null, content: null,
  title: 'Jira MCP Setup', icon: null,
  rows: [],            // current page layout: {view, h, gap} or {status: true}
  status: { text: '', kind: '' }, statusView: null,
  actions: [],         // tag -> handler
  buttons: [],         // buttons disabled while busy
  refs: {},            // named controls of the current page
  page: '', shown: false, closing: false, buf: '',
  projectRows: [], listDoc: null,
};

function emit(obj) {
  const s = $(JSON.stringify(obj) + '\n');
  $.NSFileHandle.fileHandleWithStandardOutput.writeData(s.dataUsingEncoding($.NSUTF8StringEncoding));
}

function closeServer() {
  if (ui.closing) return;
  ui.closing = true;
  emit({ action: 'close' });
  $.NSApp.terminate(null);
}

function quit() {
  ui.closing = true;
  $.NSApp.terminate(null);
}

ObjC.registerSubclass({
  name: 'JMCPHost',
  protocols: ['NSWindowDelegate'],
  methods: {
    'onData:': {
      types: ['void', ['id']],
      implementation: function (n) {
        const data = n.userInfo.objectForKey($.NSFileHandleNotificationDataItem);
        if (data.isNil() || Number(data.length) === 0) { quit(); return; } // parent went away
        ui.buf += ObjC.unwrap($.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding));
        let i;
        while ((i = ui.buf.indexOf('\n')) >= 0) {
          const line = ui.buf.slice(0, i).trim();
          ui.buf = ui.buf.slice(i + 1);
          if (!line) continue;
          try { handle(JSON.parse(line)); } catch (e) { console.log('jira-mcp window: ' + e); }
        }
        n.object.readInBackgroundAndNotify;
      },
    },
    'clicked:': {
      types: ['void', ['id']],
      implementation: function (sender) { const h = ui.actions[Number(sender.tag)]; if (h) h(); },
    },
    'filter:': {
      types: ['void', ['id']],
      implementation: function (sender) { layoutProjects(ObjC.unwrap(sender.stringValue)); },
    },
    'closeServer:': {
      types: ['void', ['id']],
      implementation: function () { closeServer(); },
    },
    'windowWillClose:': {
      types: ['void', ['id']],
      implementation: function () { closeServer(); },
    },
  },
});

// ---------- controls ----------

function label(text, opts) {
  opts = opts || {};
  const l = $.NSTextField.wrappingLabelWithString(text || '');
  l.preferredMaxLayoutWidth = opts.width || W;
  if (opts.font) l.font = opts.font;
  if (opts.color) l.textColor = opts.color;
  if (opts.selectable) l.selectable = true;
  return l;
}

function labelHeight(l, width) {
  l.preferredMaxLayoutWidth = width;
  return Math.ceil(Number(l.fittingSize.height));
}

function field(cls, value, placeholder) {
  const f = cls.alloc.initWithFrame($.NSMakeRect(0, 0, W, 24));
  f.stringValue = value || '';
  f.placeholderString = placeholder || '';
  return f;
}

function button(title, handler, opts) {
  opts = opts || {};
  const b = $.NSButton.buttonWithTitleTargetAction(title, ui.host, 'clicked:');
  b.tag = ui.actions.length;
  ui.actions.push(handler);
  if (opts.primary) b.keyEquivalent = '\r';
  if (opts.destructive) {
    b.bezelColor = $.NSColor.systemRedColor;
    const attrs = $.NSMutableDictionary.dictionary;
    attrs.setObjectForKey($.NSColor.whiteColor, $.NSForegroundColorAttributeName);
    attrs.setObjectForKey($.NSFont.boldSystemFontOfSize(13), $.NSFontAttributeName);
    // NSAttributedString initializers are not bridged to JXA; use performSelector.
    b.attributedTitle = $.NSAttributedString.alloc.performSelectorWithObjectWithObject('initWithString:attributes:', $(title), attrs);
  } else {
    ui.buttons.push(b);
  }
  b.sizeToFit;
  b.frame = $.NSMakeRect(0, 0, Math.max(Number(b.frame.size.width) + 16, 96), 32);
  return b;
}

// buttonRow places the close-server button at the far left and the other
// buttons right-aligned.
function buttonRow(buttons) {
  const v = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, W, 32));
  const close = closeButton();
  const cw = Number(close.frame.size.width);
  close.frame = $.NSMakeRect(0, 0, cw, 32);
  v.addSubview(close);
  let x = W;
  const xs = buttons.slice().reverse().map(b => {
    x -= Number(b.frame.size.width);
    const at = x;
    x -= 8;
    return at;
  }).reverse();
  buttons.forEach((b, i) => {
    b.frame = $.NSMakeRect(xs[i], 0, Number(b.frame.size.width), 32);
    v.addSubview(b);
  });
  return v;
}

function closeButton() {
  return button('Close server', closeServer, { destructive: true });
}

function header(step) {
  const v = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, W, 48));
  let x = 0;
  if (ui.icon) {
    const img = $.NSImageView.imageViewWithImage(ui.icon);
    img.imageScaling = $.NSImageScaleProportionallyUpOrDown;
    img.frame = $.NSMakeRect(0, 0, 48, 48);
    v.addSubview(img);
    x = 60;
  }
  const t = label(ui.title, { font: $.NSFont.boldSystemFontOfSize(17), width: W - x });
  t.frame = $.NSMakeRect(x, 22, W - x, 24);
  v.addSubview(t);
  const s = label(step, { font: $.NSFont.systemFontOfSize(12), color: $.NSColor.secondaryLabelColor, width: W - x });
  s.frame = $.NSMakeRect(x, 2, W - x, 18);
  v.addSubview(s);
  return v;
}

// ---------- layout ----------

function row(view, h, gap) { return { view, h, gap }; }
function textRow(text, gap, opts) {
  const l = label(text, opts);
  return row(l, labelHeight(l, W), gap);
}

function statusRow() {
  const s = ui.status;
  if (!s.text) return null;
  const colors = {
    error: [$.NSColor.systemRedColor, $.NSColor.systemRedColor.colorWithAlphaComponent(0.12)],
    ok: [$.NSColor.systemGreenColor, $.NSColor.systemGreenColor.colorWithAlphaComponent(0.12)],
    busy: [$.NSColor.separatorColor, $.NSColor.clearColor],
  }[s.kind] || [$.NSColor.separatorColor, $.NSColor.clearColor];
  const pad = 10, iconW = 22;
  const textW = W - 2 * pad - iconW;
  const font = s.kind === 'error' ? $.NSFont.boldSystemFontOfSize(13) : $.NSFont.systemFontOfSize(13);
  const l = label(s.text, { font, color: s.kind === 'error' ? $.NSColor.systemRedColor : null, selectable: s.kind !== 'busy', width: textW });
  const lh = Math.max(labelHeight(l, textW), 18);
  const h = lh + 2 * pad;
  const box = $.NSBox.alloc.initWithFrame($.NSMakeRect(0, 0, W, h));
  box.boxType = 4;          // NSBoxCustom
  box.titlePosition = 0;    // NSNoTitle
  box.borderWidth = 1;
  box.cornerRadius = 6;
  box.borderColor = colors[0];
  box.fillColor = colors[1];
  box.contentViewMargins = $.NSMakeSize(0, 0);
  const inner = box.contentView;
  if (s.kind === 'busy') {
    const sp = $.NSProgressIndicator.alloc.initWithFrame($.NSMakeRect(pad, h - pad - 17, 16, 16));
    sp.style = 1;           // NSProgressIndicatorStyleSpinning
    sp.controlSize = 1;     // small
    sp.displayedWhenStopped = false;
    inner.addSubview(sp);
    sp.startAnimation(null);
  } else {
    const name = s.kind === 'error' ? $.NSImageNameCaution : $.NSImageNameInfo;
    const img = $.NSImageView.imageViewWithImage($.NSImage.imageNamed(name));
    img.frame = $.NSMakeRect(pad, h - pad - 18, 18, 18);
    inner.addSubview(img);
  }
  l.frame = $.NSMakeRect(pad + iconW, pad, textW, lh);
  inner.addSubview(l);
  return row(box, h, 14);
}

function layout() {
  const width = W + 2 * M;
  if (ui.statusView) { ui.statusView.removeFromSuperview; ui.statusView = null; }
  const rows = [];
  ui.rows.forEach(r => {
    if (!r.status) { rows.push(r); return; }
    const s = statusRow();
    if (s) { ui.statusView = s.view; rows.push(s); }
  });
  const H = 2 * M + rows.reduce((sum, r, i) => sum + r.h + (i < rows.length - 1 ? r.gap : 0), 0);
  const fr = ui.win.frameRectForContentRect($.NSMakeRect(0, 0, width, H));
  if (ui.shown) {
    const old = ui.win.frame;
    const top = Number(old.origin.y) + Number(old.size.height);
    ui.win.setFrameDisplay($.NSMakeRect(Number(old.origin.x), top - Number(fr.size.height),
      Number(fr.size.width), Number(fr.size.height)), true);
  } else {
    ui.win.setContentSize($.NSMakeSize(width, H));
  }
  let y = H - M;
  rows.forEach(r => {
    y -= r.h;
    r.view.frame = $.NSMakeRect(M, y, W, r.h);
    if (r.view.superview.isNil() || !r.view.superview.isEqual(ui.content)) ui.content.addSubview(r.view);
    y -= r.gap;
  });
}

function newPage(name, rows, focus) {
  ui.page = name;
  ui.content = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, W + 2 * M, 100));
  ui.win.contentView = ui.content;
  ui.statusView = null;
  ui.rows = rows;
  layout();
  if (!ui.shown) {
    ui.win.center;
    presentWindow();
    ui.shown = true;
  }
  if (focus) ui.win.makeFirstResponder(focus);
}

function presentWindow() {
  ui.win.makeKeyAndOrderFront(null);
  $.NSApp.activateIgnoringOtherApps(true);
}

function setStatus(text, kind) {
  ui.status = { text: text || '', kind: kind || '' };
  if (kind !== 'busy') ui.buttons.forEach(b => { b.enabled = true; });
  if (ui.content) layout();
  if (kind === 'error') $.NSBeep();
}

function busy(text) {
  ui.buttons.forEach(b => { b.enabled = false; });
  setStatus(text, 'busy');
}

function resetPage() {
  ui.actions = [];
  ui.buttons = [];
  ui.refs = {};
}

// ---------- pages ----------

function showConnect(c) {
  resetPage();
  const url = field($.NSTextField, c.jiraUrl, 'https://jira.company.com');
  const tok = field($.NSSecureTextField, '', c.tokenHint || 'Paste your PAT');
  const ca = field($.NSTextField, c.customCaCertPath, '/path/to/corp-root-ca.pem');
  url.nextKeyView = tok; tok.nextKeyView = ca; ca.nextKeyView = url;
  const next = button('Next', () => {
    busy('Connecting to Jira and loading your projects\u2026');
    emit({
      action: 'next',
      jiraUrl: ObjC.unwrap(url.stringValue),
      personalToken: ObjC.unwrap(tok.stringValue),
      customCaCertPath: ObjC.unwrap(ca.stringValue),
    });
  }, { primary: true });
  ui.refs = { url, tok, ca, next };
  const bold = { font: $.NSFont.boldSystemFontOfSize(13) };
  ui.status = { text: c.status || '', kind: c.status ? (c.error ? 'error' : 'ok') : '' };
  newPage('connect', [
    row(header('Step 1 of 2 \u00b7 Connect'), 48, 14),
    textRow(c.message, 14),
    textRow('Jira URL', 4, bold), row(url, 24, 12),
    textRow('Personal Access Token', 4, bold), row(tok, 24, 12),
    textRow('Corporate root CA file (PEM path, optional)', 4, bold), row(ca, 24, 18),
    { status: true },
    row(buttonRow([next]), 32, 0),
  ], c.jiraUrl ? tok : url);
}

function layoutProjects(filter) {
  const q = (filter || '').trim().toLowerCase();
  const shown = ui.projectRows.filter(p => !q || p.text.includes(q));
  ui.projectRows.forEach(p => p.box.setHidden(true));
  const w = W - 20;
  const h = Math.max(shown.length * ROW + 4, LIST_H - 4);
  ui.listDoc.setFrameSize($.NSMakeSize(w, h));
  shown.forEach((p, i) => {
    p.box.frame = $.NSMakeRect(4, h - 2 - (i + 1) * ROW, w - 8, ROW);
    p.box.setHidden(false);
  });
  ui.listDoc.scrollPoint($.NSMakePoint(0, h));
  return shown.length;
}

function selectedKeys() {
  return ui.projectRows.filter(p => Number(p.box.state) === 1).map(p => p.key);
}

function showProjects(c) {
  resetPage();
  const selected = new Set(c.selected || []);
  ui.listDoc = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, W - 20, LIST_H));
  ui.projectRows = (c.projects || []).map(p => {
    const box = $.NSButton.checkboxWithTitleTargetAction(p.name ? p.key + ' \u2014 ' + p.name : p.key, null, null);
    box.state = selected.has(p.key) ? 1 : 0;
    box.lineBreakMode = $.NSLineBreakByTruncatingTail;
    ui.listDoc.addSubview(box);
    return { key: p.key, text: (p.key + ' ' + (p.name || '')).toLowerCase(), box };
  });
  const scroll = $.NSScrollView.alloc.initWithFrame($.NSMakeRect(0, 0, W, LIST_H));
  scroll.hasVerticalScroller = true;
  scroll.borderType = $.NSBezelBorder;
  scroll.documentView = ui.listDoc;
  layoutProjects('');

  const search = $.NSSearchField.alloc.initWithFrame($.NSMakeRect(0, 0, W, 24));
  search.placeholderString = 'Filter projects';
  search.target = ui.host;
  search.action = 'filter:';
  const write = $.NSButton.checkboxWithTitleTargetAction('Allow write tools (create/update issues, comments, transitions)', null, null);
  write.state = c.enableWriteTools ? 1 : 0;
  const values = () => ({ projectKeys: selectedKeys(), enableWriteTools: Number(write.state) === 1 });
  const back = button('Back', () => {
    busy('');
    emit(Object.assign({ action: 'back' }, values()));
  });
  const save = button('Save', () => {
    busy('Checking the selected projects and saving\u2026');
    emit(Object.assign({ action: 'save' }, values()));
  }, { primary: true });
  ui.refs = { search, write, back, save, scroll };
  ui.status = { text: '', kind: '' };
  newPage('projects', [
    row(header('Step 2 of 2 \u00b7 Projects'), 48, 14),
    textRow(c.message, 14),
    textRow('Allowed project(s) \u2014 JIRA_ALLOWED_PROJECTS', 4, { font: $.NSFont.boldSystemFontOfSize(13) }),
    row(search, 24, 6),
    row(scroll, LIST_H, 12),
    row(write, 18, 18),
    { status: true },
    row(buttonRow([back, save]), 32, 0),
  ], search);
}

function showRegister(c) {
  resetPage();
  const rows = [row(header('Setup complete \u00b7 Register with your IDE'), 48, 14), textRow(c.message, 14)];
  const list = [];
  (c.buttons || []).forEach((t, i) => {
    const b = button(t, () => { busy('Working\u2026'); emit({ action: 'choose', index: i }); });
    b.frame = $.NSMakeRect(0, 0, W, 32);
    list.push(b);
    rows.push(row(b, 32, 6));
  });
  if (rows.length > 2) rows[rows.length - 1].gap = 18;
  ui.refs = { choices: list };
  ui.status = { text: c.status || '', kind: c.status ? (c.error ? 'error' : 'ok') : '' };
  rows.push({ status: true }, row(buttonRow([]), 32, 0));
  newPage('register', rows, null);
}

function copyText(c) {
  const pb = $.NSPasteboard.generalPasteboard;
  pb.clearContents;
  if (pb.setStringForType($(c.text || ''), $.NSPasteboardTypeString)) setStatus(c.message, 'ok');
  else setStatus('Could not copy to the clipboard.', 'error');
}

// onUpdate is called after every handled command (integration-test hook).
function onUpdate(c) {}

function handle(c) {
  switch (c.cmd) {
    case 'init':
      ui.title = c.title || ui.title;
      ui.win.title = ui.title;
      ui.icon = loadIcon(c);
      if (ui.icon) $.NSApp.applicationIconImage = ui.icon;
      break;
    case 'connect': showConnect(c); break;
    case 'projects': showProjects(c); break;
    case 'register': showRegister(c); break;
    case 'status': setStatus(c.status, c.error ? 'error' : 'ok'); break;
    case 'copy': copyText(c); break;
    case 'quit': quit(); return;
    default: console.log('jira-mcp window: unknown command ' + c.cmd);
  }
  onUpdate(c);
}

function loadIcon(c) {
  if (c.iconFile) {
    const img = $.NSImage.alloc.initWithContentsOfFile(c.iconFile);
    if (!img.isNil()) return img;
  }
  if (c.icon) {
    const data = $.NSData.alloc.initWithBase64EncodedStringOptions(c.icon, 0);
    if (!data.isNil()) {
      const img = $.NSImage.alloc.initWithData(data);
      if (!img.isNil()) return img;
    }
  }
  return null;
}

function menuItem(menu, title, action, key, target) {
  const item = menu.addItemWithTitleActionKeyEquivalent(title, action, key);
  if (target) item.target = target;
  return item;
}

function setupMenus(app) {
  const main = $.NSMenu.alloc.init;
  const appItem = $.NSMenuItem.alloc.init;
  main.addItem(appItem);
  const appMenu = $.NSMenu.alloc.initWithTitle('Jira MCP');
  menuItem(appMenu, 'Hide Jira MCP Setup', 'hide:', 'h');
  appMenu.addItem($.NSMenuItem.separatorItem);
  menuItem(appMenu, 'Close server', 'closeServer:', 'q', ui.host);
  appItem.submenu = appMenu;

  // Without an Edit menu, Cmd+X/C/V/A do not reach the text fields.
  const editItem = $.NSMenuItem.alloc.init;
  main.addItem(editItem);
  const edit = $.NSMenu.alloc.initWithTitle('Edit');
  [['Undo', 'undo:', 'z'], ['Redo', 'redo:', 'Z'], ['Cut', 'cut:', 'x'], ['Copy', 'copy:', 'c'],
   ['Paste', 'paste:', 'v'], ['Select All', 'selectAll:', 'a']]
    .forEach(([t, a, k]) => menuItem(edit, t, a, k));
  editItem.submenu = edit;

  const winItem = $.NSMenuItem.alloc.init;
  main.addItem(winItem);
  const winMenu = $.NSMenu.alloc.initWithTitle('Window');
  menuItem(winMenu, 'Minimize', 'performMiniaturize:', 'm');
  menuItem(winMenu, 'Close', 'performClose:', 'w');
  winItem.submenu = winMenu;
  app.mainMenu = main;
  app.windowsMenu = winMenu;
}

function run() {
  const app = $.NSApplication.sharedApplication;
  // A regular app gets a Dock icon, so a minimized window can be restored.
  app.setActivationPolicy($.NSApplicationActivationPolicyRegular);
  ui.host = $.JMCPHost.alloc.init;
  setupMenus(app);
  ui.win = $.NSWindow.alloc.initWithContentRectStyleMaskBackingDefer(
    $.NSMakeRect(0, 0, W + 2 * M, 200),
    $.NSWindowStyleMaskTitled | $.NSWindowStyleMaskClosable | $.NSWindowStyleMaskMiniaturizable,
    $.NSBackingStoreBuffered, false);
  ui.win.releasedWhenClosed = false;
  ui.win.title = ui.title;
  ui.win.delegate = ui.host;

  const input = $.NSFileHandle.fileHandleWithStandardInput;
  $.NSNotificationCenter.defaultCenter.addObserverSelectorNameObject(
    ui.host, 'onData:', $.NSFileHandleReadCompletionNotification, input);
  input.readInBackgroundAndNotify;
  app.run;
  return '';
}
