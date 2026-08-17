# z9s

<div align="center">
  <p><strong>k9s for Camunda 8</strong> — a terminal UI for Zeebe process instances, definitions, and incidents.</p>

  <p>
    <a href="#install">install</a> ·
    <a href="#quick-start">quick start</a> ·
    <a href="#keys">keys</a> ·
    <a href="BACKLOG.md">roadmap</a>
  </p>

  <p>
    <a href="https://github.com/00quasr/z9s/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/00quasr/z9s/ci.yml?branch=main&label=ci" alt="CI status" /></a>
    <a href="https://github.com/00quasr/z9s/releases"><img src="https://img.shields.io/github/v/release/00quasr/z9s?label=release" alt="Latest release" /></a>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/00quasr/z9s?label=license" alt="License" /></a>
    <a href="https://github.com/00quasr/homebrew-tap"><img src="https://img.shields.io/badge/homebrew-00quasr%2Ftap-orange" alt="Homebrew tap" /></a>
    <a href="https://goreportcard.com/report/github.com/00quasr/z9s"><img src="https://goreportcard.com/badge/github.com/00quasr/z9s" alt="Go Report Card" /></a>
    <a href="https://github.com/00quasr/z9s/releases"><img src="https://img.shields.io/github/downloads/00quasr/z9s/total?label=downloads" alt="Downloads" /></a>
    <a href="https://github.com/00quasr/z9s/stargazers"><img src="https://img.shields.io/github/stars/00quasr/z9s?style=flat&label=stars" alt="GitHub stars" /></a>
  </p>
</div>

`zbctl` is bare-bones and Operate is a heavy web UI — z9s puts your orchestration
cluster in the terminal: live tables of process instances, incidents with their
error messages, one-key navigation, auto-refresh.

<img width="895" height="951" alt="Screenshot 2026-08-16 at 21 15 02" src="https://github.com/user-attachments/assets/9270ee20-78e4-48b0-87d7-d32ba537343a" />

## Install

With Homebrew (macOS / Linux):

```sh
brew install 00quasr/tap/z9s
```

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

## Connecting to real clusters

z9s reads [c8ctl](https://docs.camunda.io/docs/apis-tools/c8ctl/getting-started/)
profiles (read-only; interop tested against c8ctl v3.3.0) — any cluster added
with `c8ctl add profile` just works:

```sh
c8ctl add profile work --baseUrl https://camunda.corp.example/v2 \
  --clientId z9s --clientSecret … --oAuthUrl https://idp.corp.example/token \
  --audience camunda-api
z9s --profile work
```

Resolution order: `--profile` flag → c8ctl's active profile (`c8ctl use
profile`) → `CAMUNDA_*` env vars (when `CAMUNDA_BASE_URL` is set) → the
`local` profile → built-in fallback (`http://localhost:8080`, basic
`demo`/`demo` — c8run's default; an auth-disabled cluster ignores the
header). The header always shows which profile and auth mode are active.

Auth mode is inferred from the profile: client id + secret → OAuth
client-credentials (SaaS: token URL `https://login.cloud.camunda.io/oauth/token`,
audience `zeebe.camunda.io`, base URL `https://{region}.api.camunda.io/{clusterId}`;
Self-Managed: your IdP's token endpoint and configured audience), username +
password → HTTP Basic (8.8+ Self-Managed), neither → unauthenticated.

Env vars (c8ctl-compatible): `CAMUNDA_BASE_URL`, `CAMUNDA_CLIENT_ID`,
`CAMUNDA_CLIENT_SECRET`, `CAMUNDA_OAUTH_URL`, `CAMUNDA_TOKEN_AUDIENCE`,
`CAMUNDA_OAUTH_SCOPE`, `CAMUNDA_USERNAME`, `CAMUNDA_PASSWORD`.

Credential safety: `--addr` **alone** always connects unauthenticated to
exactly that address; combine `--addr` with `--profile` to point a profile's
credentials at a different address deliberately.

<img width="885" height="932" alt="Screenshot 2026-08-16 at 21 15 23" src="https://github.com/user-attachments/assets/f7e2d59f-13ed-4dab-a864-c12496a61289" />

## Keys

| Key | Action |
|---|---|
| `1` / `2` / `3` | Instances · Definitions · Incidents |
| `tab` | cycle views (list) / switch pane focus (detail) |
| `↑` / `↓` | move selection |
| `enter` | open instance detail (from an instance or incident row) |
| `esc` | back |
| `ctrl+r` | resolve the selected incident (restores job retries first) |
| `ctrl+k` | cancel the selected instance — asks for confirmation |
| `s` | start an instance of the selected definition |
| `r` | refresh now (auto-refresh every 5s) |
| `q` | quit |

## Instance detail

`enter` on an instance (or incident) drills into it: definition, state and
timing in the header, an incident banner with the error message, the element
instance history showing exactly where the token sits, and the instance's
variables.

## Demo data & traffic simulator

`examples/order-fulfillment.bpmn` is a realistic process — parallel
inventory/payment branches, an error boundary for declined payments, a
pickup timer, and a user task with a form. `z9s-sim` drives it like a busy
production system: instances started continuously, workers with latency,
transient failures, declined payments, and occasional exhausted-retries
incidents.

```sh
curl -X POST localhost:8080/v2/deployments \
  -F "resources=@examples/order-fulfillment.bpmn" \
  -F "resources=@examples/confirm-delivery.form"
go run ./cmd/z9s-sim --rate 20 --burst 8   # ctrl+c to stop
```

`examples/z9s-demo.bpmn` is the minimal variant: one workerless service
task (`z9s-demo-payment`), so instances just stay visibly ACTIVE.

## Status

Young but capable: views with drill-down, incident resolution, instance
cancel/start, and authentication (basic + OAuth via c8ctl profiles) against
local, Self-Managed, or SaaS clusters. See [BACKLOG.md](BACKLOG.md) for
what's next — the project is built one small step per day.
