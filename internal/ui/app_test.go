package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
)

func TestAppEnterPushesDetailScreen(t *testing.T) {
	app := NewApp(nil, "http://test")
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
