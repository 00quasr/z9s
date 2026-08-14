package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshInterval = 5 * time.Second

var (
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	tabStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Padding(0, 1)
	activeTab     = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("39")).Bold(true).Padding(0, 1)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	healthyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	incidentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	sectionStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)
)

// tickMsg drives the auto-refresh loop. The root App owns rescheduling;
// screens only react to it by returning their fetch command.
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refreshMsg asks a screen to refetch immediately (sent when it becomes
// the top of the stack again after a pop).
type refreshMsg struct{}

func newTable() table.Model {
	t := table.New(table.WithHeight(15))
	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Bold(true)
	s.Selected = s.Selected.Foreground(lipgloss.Color("16")).Background(lipgloss.Color("39")).Bold(false)
	t.SetStyles(s)
	return t
}

// setRows works around bubbles v1.0.0 pinning the cursor at -1 forever:
// SetRows with zero rows clamps it to -1, and a later SetRows with data
// never raises it back, so SelectedRow() returns nil until the user moves.
func setRows(t *table.Model, rows []table.Row) {
	t.SetRows(rows)
	if t.Cursor() < 0 && len(rows) > 0 {
		t.SetCursor(0)
	}
}

func formatTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
