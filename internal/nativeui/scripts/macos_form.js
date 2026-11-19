function run() {
  const app = setupApp();
  const alert = newAlert();
  ['Save', 'Test connection', 'Cancel'].forEach(t => alert.addButtonWithTitle(t));

  const W = 400;
  const rows = [];
  const add = (view, h, gap) => { rows.push({ view, h, gap }); return view; };
  const label = text => add($.NSTextField.labelWithString(text), 17, 3);
  const field = (cls, value, placeholder) => {
    const f = cls.alloc.initWithFrame($.NSMakeRect(0, 0, W, 24));
    f.stringValue = value;
    f.placeholderString = placeholder;
    return add(f, 24, 12);
  };

  label('Jira URL');
  const url = field($.NSTextField, env('JMCP_URL'), 'https://jira.company.com');
  label('Personal Access Token');
  const tok = field($.NSSecureTextField, '', env('JMCP_TOKEN_HINT') || 'Paste your PAT');
  label('Project key(s), comma-separated');
  const keys = field($.NSTextField, env('JMCP_KEYS'), 'PLAT,CORE');
  const write = add($.NSButton.checkboxWithTitleTargetAction(
    'Allow write tools (create/update issues, comments, transitions)', null, null), 18, 12);
  write.state = env('JMCP_WRITE') === '1' ? ON : 0;
  label('Corporate root CA file (PEM path, optional)');
  const ca = field($.NSTextField, env('JMCP_CA'), '/path/to/corp-root-ca.pem');

  const H = rows.reduce((s, r) => s + r.h + r.gap, 0) - rows[rows.length - 1].gap;
  const view = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, W, H));
  let y = H;
  rows.forEach(r => {
    y -= r.h;
    r.view.frame = $.NSMakeRect(0, y, W, r.h);
    view.addSubview(r.view);
    y -= r.gap;
  });
  url.nextKeyView = tok; tok.nextKeyView = keys; keys.nextKeyView = write; write.nextKeyView = ca; ca.nextKeyView = url;
  alert.accessoryView = view;
  alert.window.initialFirstResponder = env('JMCP_URL') ? tok : url;

  const r = runAlert(app, alert);
  const action = ['save', 'test'][r - FIRST_BUTTON] || 'cancel';
  return JSON.stringify({
    action,
    jiraUrl: ObjC.unwrap(url.stringValue),
    personalToken: ObjC.unwrap(tok.stringValue),
    projectKeys: ObjC.unwrap(keys.stringValue),
    enableWriteTools: Number(write.state) === ON,
    customCaCertPath: ObjC.unwrap(ca.stringValue),
  });
}
