package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
)

// Screen is one layer of the navigation stack (list, detail, …).
// Update returns the possibly-replaced screen, like tea.Model does.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
}

type pushMsg struct{ screen Screen }

// pushScreen is used by screens to navigate into a deeper screen.
func pushScreen(s Screen) tea.Cmd {
	return func() tea.Msg { return pushMsg{screen: s} }
}

type popMsg struct{}

// popScreen is used by screens to dismiss themselves (e.g. a declined
// confirm modal).
func popScreen() tea.Cmd {
	return func() tea.Msg { return popMsg{} }
}

// App is the root model: it owns the screen stack, global keys
// (quit, esc-to-back), the refresh tick, and window sizing.
type App struct {
	stack  []Screen
	width  int
	height int
}

func NewApp(client *camunda.Client, addr string) App {
	return App{stack: []Screen{newClusterScreen(client, addr)}}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.stack[0].Init(), tick())
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		cmds := make([]tea.Cmd, 0, len(a.stack))
		for i, s := range a.stack {
			ns, cmd := s.Update(msg)
			a.stack[i] = ns
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "esc":
			return a.pop()
		}

	case tickMsg:
		// Root owns the tick loop; only the visible screen refreshes.
		app, cmd := a.updateTop(msg)
		return app, tea.Batch(cmd, tick())

	case pushMsg:
		s, _ := msg.screen.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.stack = append(a.stack, s)
		return a, s.Init()

	case popMsg:
		return a.pop()

	case actionDoneMsg:
		// The action was launched from a confirm modal (or directly from
		// a list); either way the screen below shows the outcome.
		if len(a.stack) > 1 {
			if _, ok := a.stack[len(a.stack)-1].(*confirmScreen); ok {
				a.stack = a.stack[:len(a.stack)-1]
			}
		}
		return a.updateTop(msg)
	}

	return a.updateTop(msg)
}

func (a App) pop() (tea.Model, tea.Cmd) {
	if len(a.stack) <= 1 {
		return a, nil
	}
	a.stack = a.stack[:len(a.stack)-1]
	return a.updateTop(refreshMsg{})
}

func (a App) updateTop(msg tea.Msg) (App, tea.Cmd) {
	top := len(a.stack) - 1
	ns, cmd := a.stack[top].Update(msg)
	a.stack[top] = ns
	return a, cmd
}

func (a App) View() string {
	return a.stack[len(a.stack)-1].View()
}
