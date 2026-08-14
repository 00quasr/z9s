package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
)

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

type clusterSnapshot struct {
	topology    *camunda.Topology
	definitions []camunda.ProcessDefinition
	instances   []camunda.ProcessInstance
	incidents   []camunda.Incident
	instTotal   int
	incTotal    int
	err         error
	fetchedAt   time.Time
}

type clusterScreen struct {
	client  *camunda.Client
	addr    string
	version string

	active view
	tables [viewCount]table.Model
	snap   clusterSnapshot
	flash  string
	width  int
	height int
}

func newClusterScreen(client *camunda.Client, addr, version string) *clusterScreen {
	m := &clusterScreen{client: client, addr: addr, version: version, active: viewInstances}
	for v := view(0); v < viewCount; v++ {
		t := newTable()
		t.Focus()
		m.tables[v] = t
	}
	return m
}

func (m *clusterScreen) Init() tea.Cmd { return m.fetch }

func (m *clusterScreen) fetch() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	snap := clusterSnapshot{fetchedAt: time.Now()}
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

func (m *clusterScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for v := view(0); v < viewCount; v++ {
			m.tables[v].SetHeight(max(5, m.height-8))
		}
		m.rebuildTables()
		return m, nil
	case tickMsg, refreshMsg:
		return m, m.fetch
	case clusterSnapshot:
		m.snap = msg
		m.rebuildTables()
		return m, nil
	case actionDoneMsg:
		if msg.err != nil {
			m.flash = errStyle.Render(msg.err.Error())
		} else {
			m.flash = healthyStyle.Render(msg.note)
		}
		return m, m.fetch
	case tea.KeyMsg:
		switch msg.String() {
		case "1":
			m.active = viewInstances
			return m, nil
		case "2":
			m.active = viewDefinitions
			return m, nil
		case "3":
			m.active = viewIncidents
			return m, nil
		case "tab":
			m.active = (m.active + 1) % viewCount
			return m, nil
		case "r":
			return m, m.fetch
		case "enter":
			if key := m.selectedInstanceKey(); key != "" {
				return m, pushScreen(newDetailScreen(m.client, key))
			}
			return m, nil
		case "ctrl+k":
			if key := m.selectedInstanceKey(); key != "" && m.active == viewInstances {
				client := m.client
				return m, pushScreen(newConfirmScreen(
					"Cancel process instance "+key+"?",
					runAction("cancelled instance "+key, func(ctx context.Context) error {
						return client.CancelProcessInstance(ctx, key)
					})))
			}
			return m, nil
		case "ctrl+r":
			if in := m.selectedIncident(); in != nil {
				client, ik, jk := m.client, in.IncidentKey, in.JobKey
				return m, runAction("resolved incident "+ik, func(ctx context.Context) error {
					return client.ResolveIncident(ctx, ik, jk)
				})
			}
			return m, nil
		case "s":
			if m.active == viewDefinitions {
				if row := m.tables[viewDefinitions].SelectedRow(); row != nil {
					client, dk := m.client, row[0]
					return m, runAction("started instance of "+row[1], func(ctx context.Context) error {
						_, err := client.CreateProcessInstance(ctx, dk)
						return err
					})
				}
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.tables[m.active], cmd = m.tables[m.active].Update(msg)
	return m, cmd
}

// selectedInstanceKey resolves the process-instance key behind the
// selected row: directly on the instances view, via the owning instance
// on the incidents view.
func (m *clusterScreen) selectedInstanceKey() string {
	row := m.tables[m.active].SelectedRow()
	if row == nil {
		return ""
	}
	switch m.active {
	case viewInstances:
		return row[0]
	case viewIncidents:
		if in := m.selectedIncident(); in != nil {
			return in.ProcessInstanceKey
		}
	}
	return ""
}

func (m *clusterScreen) selectedIncident() *camunda.Incident {
	if m.active != viewIncidents {
		return nil
	}
	row := m.tables[viewIncidents].SelectedRow()
	if row == nil {
		return nil
	}
	for i := range m.snap.incidents {
		if m.snap.incidents[i].IncidentKey == row[0] {
			return &m.snap.incidents[i]
		}
	}
	return nil
}

func (m *clusterScreen) rebuildTables() {
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
	setRows(&m.tables[viewInstances], rows)

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
	setRows(&m.tables[viewDefinitions], rows)

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
	setRows(&m.tables[viewIncidents], rows)
}

func (m *clusterScreen) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(" z9s " + m.version + " "))
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

	if m.flash != "" {
		b.WriteString(m.flash)
		b.WriteString("\n")
	}
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
	hint := "enter details"
	switch m.active {
	case viewInstances:
		hint += " · ctrl+k cancel"
	case viewDefinitions:
		hint = "s start instance"
	case viewIncidents:
		hint += " · ctrl+r resolve"
	}
	b.WriteString(dimStyle.Render("  ·  " + hint + " · 1/2/3 switch · tab cycle · r refresh · q quit"))
	return b.String()
}
