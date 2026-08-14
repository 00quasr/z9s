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

type detailSnapshot struct {
	key       string
	instance  *camunda.ProcessInstance
	elements  []camunda.ElementInstance
	variables []camunda.Variable
	incidents []camunda.Incident
	err       error
	fetchedAt time.Time
}

type detailPane int

const (
	paneElements detailPane = iota
	paneVariables
)

type detailScreen struct {
	client *camunda.Client
	key    string

	snap      detailSnapshot
	elements  table.Model
	variables table.Model
	focus     detailPane
	flash     string
	width     int
	height    int
}

func newDetailScreen(client *camunda.Client, processInstanceKey string) *detailScreen {
	m := &detailScreen{client: client, key: processInstanceKey, focus: paneElements}
	m.elements = newTable()
	m.elements.Focus()
	m.variables = newTable()
	return m
}

func (m *detailScreen) Init() tea.Cmd { return m.fetch }

func (m *detailScreen) fetch() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	snap := detailSnapshot{key: m.key, fetchedAt: time.Now()}
	var err error
	if snap.instance, err = m.client.GetProcessInstance(ctx, m.key); err != nil {
		snap.err = err
		return snap
	}
	if snap.elements, _, err = m.client.SearchElementInstances(ctx, m.key); err != nil {
		snap.err = err
		return snap
	}
	if snap.variables, _, err = m.client.SearchVariables(ctx, m.key); err != nil {
		snap.err = err
		return snap
	}
	if snap.incidents, _, err = m.client.SearchInstanceIncidents(ctx, m.key); err != nil {
		snap.err = err
		return snap
	}
	return snap
}

func (m *detailScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.rebuildTables()
		return m, nil
	case tickMsg, refreshMsg:
		return m, m.fetch
	case detailSnapshot:
		if msg.key != m.key {
			return m, nil
		}
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
		case "tab":
			m.setFocus((m.focus + 1) % 2)
			return m, nil
		case "r":
			return m, m.fetch
		case "ctrl+r":
			if len(m.snap.incidents) > 0 {
				client, in := m.client, m.snap.incidents[0]
				return m, runAction("resolved incident "+in.IncidentKey, func(ctx context.Context) error {
					return client.ResolveIncident(ctx, in.IncidentKey, in.JobKey)
				})
			}
			return m, nil
		case "ctrl+k":
			client, key := m.client, m.key
			return m, pushScreen(newConfirmScreen(
				"Cancel process instance "+key+"?",
				runAction("cancelled instance "+key, func(ctx context.Context) error {
					return client.CancelProcessInstance(ctx, key)
				})))
		}
	}
	var cmd tea.Cmd
	if m.focus == paneElements {
		m.elements, cmd = m.elements.Update(msg)
	} else {
		m.variables, cmd = m.variables.Update(msg)
	}
	return m, cmd
}

func (m *detailScreen) setFocus(p detailPane) {
	m.focus = p
	if p == paneElements {
		m.elements.Focus()
		m.variables.Blur()
	} else {
		m.elements.Blur()
		m.variables.Focus()
	}
}

func (m *detailScreen) rebuildTables() {
	w := max(m.width, 100)
	// Fixed chrome: header, instance line, incident banner, two section
	// labels, footer, spacing — split the rest between the two tables.
	avail := max(10, m.height-11-len(m.snap.incidents))
	elemH := max(4, avail*2/3)
	varH := max(3, avail-elemH)
	m.elements.SetHeight(elemH)
	m.variables.SetHeight(varH)

	m.elements.SetColumns([]table.Column{
		{Title: "ELEMENT", Width: 24},
		{Title: "TYPE", Width: 18},
		{Title: "STATE", Width: 11},
		{Title: "INCIDENT", Width: 9},
		{Title: "STARTED", Width: 20},
		{Title: "ENDED", Width: w - 88},
	})
	rows := make([]table.Row, 0, len(m.snap.elements))
	for _, el := range m.snap.elements {
		name := el.ElementName
		if name == "" {
			name = el.ElementID
		}
		inc := ""
		if el.HasIncident {
			inc = "!"
		}
		ended := ""
		if el.EndDate != nil {
			ended = formatTime(*el.EndDate)
		}
		rows = append(rows, table.Row{name, el.Type, el.State, inc, formatTime(el.StartDate), ended})
	}
	setRows(&m.elements, rows)

	m.variables.SetColumns([]table.Column{
		{Title: "NAME", Width: 28},
		{Title: "VALUE", Width: w - 54},
		{Title: "SCOPE", Width: 20},
	})
	rows = make([]table.Row, 0, len(m.snap.variables))
	for _, v := range m.snap.variables {
		val := v.Value
		if v.IsTruncated {
			val += " …(truncated)"
		}
		rows = append(rows, table.Row{v.Name, val, v.ScopeKey})
	}
	setRows(&m.variables, rows)
}

func (m *detailScreen) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(" z9s "))
	b.WriteString(dimStyle.Render("▸ instance "))
	b.WriteString(headerStyle.Render(m.key))
	b.WriteString("\n")

	if pi := m.snap.instance; pi != nil {
		state := pi.State
		if pi.State == "ACTIVE" {
			state = healthyStyle.Render(state)
		} else {
			state = dimStyle.Render(state)
		}
		line := fmt.Sprintf(" %s v%d · ", pi.ProcessDefinitionID, pi.ProcessDefinitionVersion)
		b.WriteString(dimStyle.Render(line))
		b.WriteString(state)
		b.WriteString(dimStyle.Render(" · started " + formatTime(pi.StartDate)))
		if pi.EndDate != nil {
			b.WriteString(dimStyle.Render(" · ended " + formatTime(*pi.EndDate)))
		}
		b.WriteString("\n")
	} else {
		b.WriteString(dimStyle.Render(" loading…\n"))
	}

	for _, in := range m.snap.incidents {
		b.WriteString(incidentStyle.Render(fmt.Sprintf(" ! %s @ %s — %s", in.ErrorType, in.ElementID, in.ErrorMessage)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(m.sectionLabel("Elements", paneElements))
	b.WriteString("\n")
	b.WriteString(m.elements.View())
	b.WriteString("\n")
	b.WriteString(m.sectionLabel("Variables", paneVariables))
	b.WriteString("\n")
	b.WriteString(m.variables.View())
	b.WriteString("\n")

	if m.flash != "" {
		b.WriteString(m.flash)
		b.WriteString("\n")
	}
	if m.snap.err != nil {
		b.WriteString(errStyle.Render(truncateLine("error: "+m.snap.err.Error(), m.width)))
	} else if !m.snap.fetchedAt.IsZero() {
		b.WriteString(dimStyle.Render("refreshed " + m.snap.fetchedAt.Format("15:04:05")))
	}
	hint := "  ·  esc back · tab focus"
	if len(m.snap.incidents) > 0 {
		hint += " · ctrl+r resolve"
	}
	b.WriteString(dimStyle.Render(hint + " · ctrl+k cancel · r refresh · q quit"))
	return b.String()
}

func (m *detailScreen) sectionLabel(name string, p detailPane) string {
	if m.focus == p {
		return activeTab.Render(name)
	}
	return sectionStyle.Render(" " + name)
}
