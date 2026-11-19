//go:build darwin

package nativeui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/internal/jira-mcp/internal/jira"
	"github.com/internal/jira-mcp/internal/setupui"
)

// testDriver is appended to the real window script. It keeps the window off
// screen, leaves the user's clipboard alone and plays the user: after each
// command it inspects the UI (reporting through a "probe" event whose jiraUrl
// carries JSON) and clicks the next button.
const testDriver = `
function presentWindow() {}
function copyText(c) { setStatus(c.message, 'ok'); }
function probe(obj) { emit({ action: 'probe', jiraUrl: JSON.stringify(obj) }); }
function rowButtons() {
  const v = ui.rows[ui.rows.length - 1].view.subviews;
  const out = [];
  for (let i = 0; i < Number(v.count); i++) out.push(ObjC.unwrap(v.objectAtIndex(i).title));
  return out;
}
function onUpdate(c) {
  if (c.cmd === 'connect') {
    ui.refs.tok.stringValue = 'typed-\u00fc';
    ui.refs.next.performClick(null);
  } else if (c.cmd === 'status' && ui.page === 'connect') {
    probe({ kind: ui.status.kind, text: ui.status.text, nextEnabled: !!ui.refs.next.enabled,
      shown: !ui.statusView.isNil(), url: ObjC.unwrap(ui.refs.url.stringValue), buttons: rowButtons() });
  } else if (c.cmd === 'projects') {
    const n = layoutProjects('pla');
    ui.projectRows.forEach(p => { if (!p.box.isHidden) p.box.state = 1; });
    layoutProjects('');
    ui.projectRows[0].box.state = 0;
    probe({ filtered: n, buttons: rowButtons() });
    ui.refs.save.performClick(null);
  } else if (c.cmd === 'register') {
    probe({ buttons: rowButtons(), choices: ui.refs.choices.map(b => ObjC.unwrap(b.title)), text: ui.status.text, kind: ui.status.kind });
    ui.refs.choices[1].performClick(null);
  } else if (c.cmd === 'copy') {
    probe({ text: ui.status.text, kind: ui.status.kind });
    const v = ui.rows[ui.rows.length - 1].view.subviews;
    v.objectAtIndex(Number(v.count) - 1).performClick(null);
  }
}`

type probeState struct {
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	NextEnabled bool     `json:"nextEnabled"`
	Shown       bool     `json:"shown"`
	URL         string   `json:"url"`
	Buttons     []string `json:"buttons"`
	Choices     []string `json:"choices"`
	Filtered    int      `json:"filtered"`
}

func TestMacOSWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("runs osascript")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := open(ctx, osascript(macWindow+testDriver), command{Cmd: "init", Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	next := func(want Action) Event {
		t.Helper()
		ev, err := p.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ev.Action != want {
			t.Fatalf("event = %#v, want %s", ev, want)
		}
		return ev
	}
	probe := func() probeState {
		t.Helper()
		var s probeState
		if err := json.Unmarshal([]byte(next("probe").JiraURL), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(p.ShowConnect(ConnectPage{Message: "M", JiraURL: "https://j", CustomCACertPath: "/ca"}))
	ev := next(ActionNext)
	if ev.JiraURL != "https://j" || ev.PersonalToken != "typed-ü" || ev.CustomCACertPath != "/ca" {
		t.Errorf("next = %#v", ev)
	}

	must(p.Status("Error: proxy refused", true))
	s := probe()
	if s.Kind != "error" || s.Text != "Error: proxy refused" || !s.NextEnabled || !s.Shown || s.URL != "https://j" ||
		!slices.Equal(s.Buttons, []string{"Close server", "Next"}) {
		t.Errorf("connect page after error = %#v", s)
	}

	must(p.ShowProjects(ProjectsPage{Message: "M", EnableWriteTools: true, Selected: []string{"CORE"},
		Projects: []jira.Project{{Key: "CORE", Name: "Core"}, {Key: "PLAT", Name: "Platform"}, {Key: "OPS"}}}))
	s = probe()
	if s.Filtered != 1 || !slices.Equal(s.Buttons, []string{"Close server", "Back", "Save"}) {
		t.Errorf("projects page = %#v", s)
	}
	ev = next(ActionSave)
	if !slices.Equal(ev.ProjectKeys, []string{"PLAT"}) || !ev.EnableWriteTools {
		t.Errorf("save = %#v", ev)
	}

	must(p.ShowRegister(RegisterPage{Message: "M", Status: "Saved.", Buttons: []string{"Install in X", "Copy VS Code config"}}))
	s = probe()
	if !slices.Equal(s.Buttons, []string{"Close server"}) || !slices.Equal(s.Choices, []string{"Install in X", "Copy VS Code config"}) ||
		s.Kind != "ok" || s.Text != "Saved." {
		t.Errorf("register page = %#v", s)
	}
	if ev = next(ActionChoose); ev.Index != 1 {
		t.Errorf("choose = %#v", ev)
	}

	must(p.Copy("snippet", "Copied."))
	if s = probe(); s.Text != "Copied." || s.Kind != "ok" {
		t.Errorf("after copy = %#v", s)
	}
	next(ActionClose) // the driver clicked "Close server"
	select {
	case <-p.Closed():
	case <-time.After(10 * time.Second):
		t.Fatal("window did not close")
	}
	if ev, err := p.Next(ctx); err != nil || ev.Action != ActionClose {
		t.Errorf("after close: %#v, %v", ev, err)
	}
}

func TestMacOSWindowInitAndQuit(t *testing.T) {
	if testing.Short() {
		t.Skip("runs osascript")
	}
	p, err := open(context.Background(), osascript(macWindow), command{Cmd: "init", Title: Title,
		Icon: base64.StdEncoding.EncodeToString(setupui.IconPNG())})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if p.exitErr != nil || p.userClosed.Load() {
		t.Errorf("exit = %v, userClosed = %v", p.exitErr, p.userClosed.Load())
	}
}
