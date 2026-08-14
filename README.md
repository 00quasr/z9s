# z9s

> k9s for Camunda 8. A terminal UI for Zeebe process instances, definitions, and incidents.

`zbctl` is bare-bones and Operate is a heavy web UI — z9s puts your orchestration
cluster in the terminal: live tables of process instances, incidents with their
error messages, one-key navigation, auto-refresh.

## Quick start

Requires Go ≥ 1.22 and a running Camunda 8.8+ cluster with the v2 REST API
(e.g. `c8run` on `localhost:8080` — the default).

```sh
go run ./cmd/z9s                          # connect to http://localhost:8080
go run ./cmd/z9s --addr http://host:8080  # any other cluster
go run ./cmd/z9s --dump                   # one plain-text snapshot, no TUI
```

## Keys

| Key | Action |
|---|---|
| `1` / `2` / `3` | Instances · Definitions · Incidents |
| `tab` | cycle views |
| `↑` / `↓` | move selection |
| `r` | refresh now (auto-refresh every 5s) |
| `q` | quit |

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
