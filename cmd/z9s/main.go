// z9s — a k9s-style terminal UI for Camunda 8 / Zeebe.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
	"github.com/00quasr/z9s/internal/ui"
)

func main() {
	addr := flag.String("addr", "http://localhost:8080", "base URL of the Camunda 8 Orchestration Cluster REST API")
	dump := flag.Bool("dump", false, "print one snapshot as plain text and exit (no TUI)")
	flag.Parse()

	client := camunda.NewClient(*addr)

	if *dump {
		if err := dumpSnapshot(client, *addr); err != nil {
			fmt.Fprintln(os.Stderr, "z9s:", err)
			os.Exit(1)
		}
		return
	}

	p := tea.NewProgram(ui.NewApp(client, *addr), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "z9s:", err)
		os.Exit(1)
	}
}

func dumpSnapshot(client *camunda.Client, addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	topo, err := client.Topology(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("cluster %s · gateway %s · %d broker(s) · %d partition(s)\n",
		addr, topo.GatewayVersion, topo.ClusterSize, topo.PartitionsCount)

	defs, _, err := client.SearchProcessDefinitions(ctx, 100)
	if err != nil {
		return err
	}
	fmt.Printf("\nprocess definitions (%d):\n", len(defs))
	for _, d := range defs {
		fmt.Printf("  %-22s %-24s v%-3d %s\n", d.ProcessDefinitionKey, d.ProcessDefinitionID, d.Version, d.ResourceName)
	}

	instances, total, err := client.SearchProcessInstances(ctx, 100)
	if err != nil {
		return err
	}
	fmt.Printf("\nprocess instances (%d):\n", total)
	for _, pi := range instances {
		inc := ""
		if pi.HasIncident {
			inc = " [incident]"
		}
		fmt.Printf("  %-22s %-24s v%-3d %-10s %s%s\n",
			pi.ProcessInstanceKey, pi.ProcessDefinitionID, pi.ProcessDefinitionVersion, pi.State, pi.StartDate, inc)
	}

	incidents, total, err := client.SearchIncidents(ctx, 100)
	if err != nil {
		return err
	}
	fmt.Printf("\nincidents (%d):\n", total)
	for _, in := range incidents {
		fmt.Printf("  %-22s %-20s %-18s %-16s %s\n",
			in.IncidentKey, in.ProcessDefinitionID, in.ElementID, in.ErrorType, in.ErrorMessage)
	}
	return nil
}
