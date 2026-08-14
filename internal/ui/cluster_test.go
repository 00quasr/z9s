package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
)

func TestEnterOnInstancePushesDetail(t *testing.T) {
	m := newClusterScreen(nil, "http://test", "dev")
	// The initial resize arrives before any data in the real app; with
	// bubbles v1.0.0 this pinned the table cursor at -1 (see setRows).
	s, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 38})
	m = s.(*clusterScreen)
	s, _ = m.Update(clusterSnapshot{
		instances: []camunda.ProcessInstance{{ProcessInstanceKey: "123", ProcessDefinitionID: "demo", State: "ACTIVE"}},
	})
	m = s.(*clusterScreen)

	if got := m.selectedInstanceKey(); got != "123" {
		t.Fatalf("selectedInstanceKey = %q, want 123 (selected row: %#v)", got, m.tables[viewInstances].SelectedRow())
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	if _, ok := cmd().(pushMsg); !ok {
		t.Fatalf("enter command produced %T, want pushMsg", cmd())
	}
}
