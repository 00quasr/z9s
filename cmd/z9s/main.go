// z9s — a k9s-style terminal UI for Camunda 8 / Zeebe.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/00quasr/z9s/internal/camunda"
	"github.com/00quasr/z9s/internal/config"
	"github.com/00quasr/z9s/internal/ui"
)

// Injected by GoReleaser via ldflags; "dev" for plain go build/install.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	addr := flag.String("addr", "", "cluster base URL; alone = unauthenticated connection, with --profile = address override keeping that profile's auth (default: resolved from profile)")
	profileFlag := flag.String("profile", "", "c8ctl profile to connect with (default: session active profile, then \"local\")")
	dump := flag.Bool("dump", false, "print one snapshot as plain text and exit (no TUI)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("z9s %s (%s, %s)\n", version, commit, date)
		return
	}

	prof, warnings, err := config.Resolve(*profileFlag, *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "z9s:", err)
		os.Exit(1)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "z9s:", w)
	}

	var rt http.RoundTripper
	switch prof.Auth {
	case config.AuthBasic:
		rt = camunda.BasicAuthTransport(prof.Username, prof.Password)
	case config.AuthOAuth:
		rt = camunda.OAuthTransport(prof.OAuthURL, prof.ClientID, prof.ClientSecret, prof.Audience, prof.Scope)
	}
	client := camunda.NewClient(prof.BaseURL, rt)

	if *dump {
		if label := prof.Label(); label != "" {
			fmt.Fprintf(os.Stderr, "z9s: profile %s [%s]\n", label, prof.Source)
		}
		if err := dumpSnapshot(client, prof.BaseURL); err != nil {
			fmt.Fprintln(os.Stderr, "z9s:", err)
			os.Exit(1)
		}
		return
	}

	p := tea.NewProgram(ui.NewApp(client, prof.BaseURL, version, prof.Label()), tea.WithAltScreen())
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
