// Package ui implements the z9s terminal interface.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00quasr/z9s/internal/camunda"
)

const refreshInterval = 5 * time.Second

type view int

const (
	viewInstances view = iota
	viewDefinitions
	viewIncidents
	viewCount
)

func (v view) title() string {
	switch v {
	case viewInstances:
		return "Instances"
	case viewDefinitions:
		return "Definitions"
	case viewIncidents:
		return "Incidents"
	}
	return ""
}

var (
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	tabStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Padding(0, 1)
	activeTab     = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("39")).Bold(true).Padding(0, 1)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	healthyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	incidentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
)

type snapshot struct {
	topology    *camunda.Topology
	definitions []camunda.ProcessDefinition
	instances   []camunda.ProcessInstance
	incidents   []camunda.Incident
	instTotal   int
	incTotal    int
	err         error
	fetchedAt   time.Time
}

type tickMsg time.Time

type Model struct {
	client *camunda.Client
	addr   string

	active view
	tables [viewCount]table.Model
	snap   snapshot
	width  int
	height int
}

func NewModel(client *camunda.Client, addr string) Model {
	m := Model{client: client, addr: addr, active: viewInstances}
	for v := view(0); v < viewCount; v++ {
		t := table.New(table.WithFocused(true), table.WithHeight(15))
		s := table.DefaultStyles()
		s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Bold(true)
		s.Selected = s.Selected.Foreground(lipgloss.Color("16")).Background(lipgloss.Color("39")).Bold(false)
		t.SetStyles(s)
		m.tables[v] = t
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetch, tick())
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) fetch() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	snap := snapshot{fetchedAt: time.Now()}
	var err error
	if snap.topology, err = m.client.Topology(ctx); err != nil {
		snap.err = err
		return snap
	}
	if snap.definitions, _, err = m.client.SearchProcessDefinitions(ctx, 100); err != nil {
		snap.err = err
		return snap
	}
	if snap.instances, snap.instTotal, err = m.client.SearchProcessInstances(ctx, 100); err != nil {
		snap.err = err
		return snap
	}
	if snap.incidents, snap.incTotal, err = m.client.SearchIncidents(ctx, 100); err != nil {
		snap.err = err
		return snap
	}
	return snap
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for v := view(0); v < viewCount; v++ {
			m.tables[v].SetHeight(max(5, m.height-8))
		}
		m.rebuildTables()
	case tickMsg:
		return m, tea.Batch(m.fetch, tick())
	case snapshot:
		m.snap = msg
		m.rebuildTables()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.active = viewInstances
		case "2":
			m.active = viewDefinitions
		case "3":
			m.active = viewIncidents
		case "tab":
			m.active = (m.active + 1) % viewCount
		case "r":
			return m, m.fetch
		}
	}
	var cmd tea.Cmd
	m.tables[m.active], cmd = m.tables[m.active].Update(msg)
	return m, cmd
}

func (m *Model) rebuildTables() {
	w := max(m.width, 100)

	m.tables[viewInstances].SetColumns([]table.Column{
		{Title: "KEY", Width: 20},
		{Title: "DEFINITION", Width: 24},
		{Title: "VER", Width: 4},
		{Title: "STATE", Width: 10},
		{Title: "INCIDENT", Width: 9},
		{Title: "STARTED", Width: w - 75},
	})
	rows := make([]table.Row, 0, len(m.snap.instances))
	for _, pi := range m.snap.instances {
		inc := ""
		if pi.HasIncident {
			inc = "!"
		}
		rows = append(rows, table.Row{
			pi.ProcessInstanceKey, pi.ProcessDefinitionID,
			fmt.Sprint(pi.ProcessDefinitionVersion), pi.State, inc, formatTime(pi.StartDate),
		})
	}
	m.tables[viewInstances].SetRows(rows)

	m.tables[viewDefinitions].SetColumns([]table.Column{
		{Title: "KEY", Width: 20},
		{Title: "ID", Width: 24},
		{Title: "NAME", Width: 28},
		{Title: "VER", Width: 4},
		{Title: "RESOURCE", Width: w - 84},
	})
	rows = make([]table.Row, 0, len(m.snap.definitions))
	for _, pd := range m.snap.definitions {
		rows = append(rows, table.Row{
			pd.ProcessDefinitionKey, pd.ProcessDefinitionID, pd.Name,
			fmt.Sprint(pd.Version), pd.ResourceName,
		})
	}
	m.tables[viewDefinitions].SetRows(rows)

	m.tables[viewIncidents].SetColumns([]table.Column{
		{Title: "KEY", Width: 20},
		{Title: "DEFINITION", Width: 20},
		{Title: "ELEMENT", Width: 18},
		{Title: "TYPE", Width: 16},
		{Title: "MESSAGE", Width: w - 82},
	})
	rows = make([]table.Row, 0, len(m.snap.incidents))
	for _, in := range m.snap.incidents {
		rows = append(rows, table.Row{
			in.IncidentKey, in.ProcessDefinitionID, in.ElementID, in.ErrorType, in.ErrorMessage,
		})
	}
	m.tables[viewIncidents].SetRows(rows)
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(" z9s "))
	if t := m.snap.topology; t != nil {
		health := healthyStyle.Render("healthy")
		for _, br := range t.Brokers {
			for _, p := range br.Partitions {
				if p.Health != "healthy" {
					health = errStyle.Render(p.Health)
				}
			}
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %s · gateway %s · %d broker(s) · %d partition(s) · ",
			m.addr, t.GatewayVersion, t.ClusterSize, t.PartitionsCount)))
		b.WriteString(health)
	} else {
		b.WriteString(dimStyle.Render(" " + m.addr + " · connecting…"))
	}
	b.WriteString("\n\n")

	for v := view(0); v < viewCount; v++ {
		label := fmt.Sprintf("%d %s", v+1, v.title())
		switch v {
		case viewInstances:
			label += fmt.Sprintf(" (%d)", m.snap.instTotal)
		case viewDefinitions:
			label += fmt.Sprintf(" (%d)", len(m.snap.definitions))
		case viewIncidents:
			label += fmt.Sprintf(" (%d)", m.snap.incTotal)
		}
		if v == m.active {
			b.WriteString(activeTab.Render(label))
		} else {
			b.WriteString(tabStyle.Render(label))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n")

	b.WriteString(m.tables[m.active].View())
	b.WriteString("\n")

	if m.snap.err != nil {
		b.WriteString(errStyle.Render("error: " + m.snap.err.Error()))
	} else if !m.snap.fetchedAt.IsZero() {
		status := fmt.Sprintf("refreshed %s", m.snap.fetchedAt.Format("15:04:05"))
		if m.snap.incTotal > 0 {
			status = incidentStyle.Render(fmt.Sprintf("%d active incident(s)", m.snap.incTotal)) + dimStyle.Render(" · "+status)
		} else {
			status = dimStyle.Render(status)
		}
		b.WriteString(status)
	}
	b.WriteString(dimStyle.Render("  ·  1/2/3 switch · tab cycle · r refresh · q quit"))
	return b.String()
}

func formatTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
