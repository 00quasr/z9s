package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
)

func TestAppEnterPushesDetailScreen(t *testing.T) {
	app := NewApp(nil, "http://test", "dev")
	model, _ := app.Update(clusterSnapshot{
		instances: []camunda.ProcessInstance{{ProcessInstanceKey: "123", ProcessDefinitionID: "demo", State: "ACTIVE"}},
	})
	app = model.(App)

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(App)
	if cmd == nil {
		t.Fatal("enter produced no command at app level")
	}
	model, _ = app.Update(cmd())
	app = model.(App)
	if len(app.stack) != 2 {
		t.Fatalf("stack depth = %d, want 2", len(app.stack))
	}
	if _, ok := app.stack[1].(*detailScreen); !ok {
		t.Fatalf("top of stack is %T, want *detailScreen", app.stack[1])
	}
}

func TestConfirmModalFlow(t *testing.T) {
	app := NewApp(nil, "http://test", "dev")
	fired := false
	action := func() tea.Msg { fired = true; return actionDoneMsg{note: "done"} }

	model, _ := app.Update(pushMsg{screen: newConfirmScreen("Sure?", action)})
	app = model.(App)
	if len(app.stack) != 2 {
		t.Fatalf("stack depth = %d, want 2 after push", len(app.stack))
	}

	// "n" declines: modal pops without running the action.
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = model.(App)
	model, _ = app.Update(cmd())
	app = model.(App)
	if len(app.stack) != 1 || fired {
		t.Fatalf("after decline: depth=%d fired=%v, want 1/false", len(app.stack), fired)
	}

	// "y" confirms: action runs, actionDoneMsg pops the modal.
	model, _ = app.Update(pushMsg{screen: newConfirmScreen("Sure?", action)})
	app = model.(App)
	model, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	app = model.(App)
	msg := cmd()
	if !fired {
		t.Fatal("confirm did not run the action")
	}
	model, _ = app.Update(msg)
	app = model.(App)
	if len(app.stack) != 1 {
		t.Fatalf("after actionDoneMsg: depth=%d, want 1", len(app.stack))
	}
}
