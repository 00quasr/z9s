package ui

import (
	"context"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// runAction wraps a cluster mutation in a command that reports an
// actionDoneMsg with the given success note.
func runAction(note string, fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := fn(ctx); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{note: note}
	}
}

// actionDoneMsg reports the outcome of a cluster-mutating action
// (cancel, resolve, start). The App pops any confirm modal when it
// arrives and forwards it to the screen below, which shows it as a
// flash message and refreshes.
type actionDoneMsg struct {
	note string
	err  error
}

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("208")).
	Padding(1, 3)

// confirmScreen is a modal layer guarding a destructive action.
type confirmScreen struct {
	prompt string
	action tea.Cmd

	busy   bool
	width  int
	height int
}

func newConfirmScreen(prompt string, action tea.Cmd) *confirmScreen {
	return &confirmScreen{prompt: prompt, action: action}
}

func (m *confirmScreen) Init() tea.Cmd { return nil }

func (m *confirmScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		switch msg.String() {
		case "y", "enter":
			m.busy = true
			return m, m.action
		case "n":
			return m, popScreen()
		}
	}
	return m, nil
}

func (m *confirmScreen) View() string {
	body := m.prompt + "\n\n"
	if m.busy {
		body += dimStyle.Render("working…")
	} else {
		body += healthyStyle.Render("[y] yes") + "   " + dimStyle.Render("[n] no")
	}
	return lipgloss.Place(max(m.width, 40), max(m.height, 10),
		lipgloss.Center, lipgloss.Center, modalStyle.Render(body))
}
