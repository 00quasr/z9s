# z9s

> k9s for Camunda 8. A terminal UI for Zeebe process instances, definitions, and incidents.

`zbctl` is bare-bones and Operate is a heavy web UI — z9s puts your orchestration
cluster in the terminal: live tables of process instances, incidents with their
error messages, one-key navigation, auto-refresh.

## Install

With Go ≥ 1.22:

```sh
go install github.com/00quasr/z9s/cmd/z9s@latest
```

Without Go: prebuilt binaries for macOS, Linux, and Windows are attached to
each [release](https://github.com/00quasr/z9s/releases) — download, unpack,
put `z9s` on your PATH.

Working on z9s itself: `go install ./cmd/z9s` from the repo root.

## Quick start

Point it at a running Camunda 8.8+ cluster with the v2 REST API
(e.g. `c8run` on `localhost:8080` — the default):

```sh
z9s                          # connect to http://localhost:8080
z9s --addr http://host:8080  # any other cluster
z9s --dump                   # one plain-text snapshot, no TUI
```

## Keys

| Key | Action |
|---|---|
| `1` / `2` / `3` | Instances · Definitions · Incidents |
| `tab` | cycle views (list) / switch pane focus (detail) |
| `↑` / `↓` | move selection |
| `enter` | open instance detail (from an instance or incident row) |
| `esc` | back |
| `r` | refresh now (auto-refresh every 5s) |
| `q` | quit |

## Instance detail

`enter` on an instance (or incident) drills into it: definition, state and
timing in the header, an incident banner with the error message, the element
instance history showing exactly where the token sits, and the instance's
variables.

## Demo data

`examples/z9s-demo.bpmn` is a tiny order process whose service task
(`z9s-demo-payment`) has no worker, so instances stay visibly ACTIVE:

```sh
curl -X POST localhost:8080/v2/deployments -F "resources=@examples/z9s-demo.bpmn"
curl -X POST localhost:8080/v2/process-instances \
  -H 'Content-Type: application/json' \
  -d '{"processDefinitionId":"z9s-demo","variables":{"orderId":"ORD-1","amount":250}}'
```

## Status

Early. Read-only views of instances, definitions, and incidents against an
unauthenticated local cluster. See [BACKLOG.md](BACKLOG.md) for what's next —
the project is built one small step per day.
