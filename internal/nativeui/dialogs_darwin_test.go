//go:build darwin

package nativeui

import (
	"context"
	"testing"
	"time"

	"github.com/internal/jira-mcp/internal/settings"
)

// pressButton replaces the modal loop: it types a token into the secure field
// and "clicks" the given button, so the real osascript backend can be tested
// without user interaction.
const pressButton = `
function runAlert(app, alert) {
  alert.layout;
  const v = alert.accessoryView;
  if (!v.isNil()) v.subviews.objectAtIndex(3).stringValue = 'typed-ü';
  return FIRST_BUTTON + Number(env('JMCP_TEST_BUTTON'));
}`

func TestMacOSBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("runs osascript")
	}
	ui, err := New([]byte{0x89, 'P', 'N', 'G'})
	if err != nil {
		t.Fatal(err)
	}
	d := ui.(*scriptDialogs)
	d.formScript += pressButton
	d.chooseScript += pressButton
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Setenv("JMCP_TEST_BUTTON", "1")
	req := FormRequest{Title: "T", Message: "M", Values: settings.Settings{JiraURL: "https://j", ProjectKeys: "PLAT", EnableWriteTools: true}}
	res, err := d.Form(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != ActionTest || res.Values.JiraURL != "https://j" || res.Values.PersonalToken != "typed-ü" ||
		res.Values.ProjectKeys != "PLAT" || !res.Values.EnableWriteTools {
		t.Errorf("form result = %#v", res)
	}

	t.Setenv("JMCP_TEST_BUTTON", "2")
	if i, err := d.Choose(ctx, "T", "M", []string{"Done", "A", "B"}); err != nil || i != 2 {
		t.Errorf("choose = %d, %v", i, err)
	}
}
