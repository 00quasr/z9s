# Backlog

Groomed, ticket-sized units — each one is a single evening's contribution.
Pick the top unchecked item; if it feels too big, split it first and commit the split.
Once the repo is on GitHub, migrate these to issues.

## Drill-down (the core loop)

- [x] `enter` on an instance opens a detail view: element instance tree (`POST /v2/element-instances/search` filtered by `processInstanceKey`)
- [x] Detail view: show instance variables (`POST /v2/variables/search`)
- [x] `enter` on an incident jumps to its process instance detail
- [x] `esc` navigates back from detail to the list

## Actions (what Operate makes you click for)

- [x] `ctrl+r` on an incident: resolve it (`PATCH /v2/jobs/{jobKey}` restores retries, then `POST /v2/incidents/{incidentKey}/resolution`)
- [x] `ctrl+k` on an instance: cancel it, with a y/n confirm prompt
- [x] `s` on a definition: start an instance (empty variables first; variable editor later)
- [ ] Variable editor when starting an instance

## Filtering & search

- [ ] `/` filter: fuzzy-match rows in the current view client-side
- [ ] Server-side filter by state (ACTIVE / COMPLETED / TERMINATED) via the search `filter` object
- [ ] Sort instances by start date descending (search `sort` param)
- [ ] Cursor pagination: load more rows on scroll past the last row (`page.endCursor` → `after`)

## Connectivity & config

- [x] Named clusters via read-only c8ctl profile interop (`--profile`, session active profile, `CAMUNDA_*` env)
- [x] Auth for SaaS / Self-Managed: OAuth client-credentials + HTTP Basic, inferred per profile
- [ ] In-TUI profile switcher
- [ ] Graceful reconnect banner when the cluster goes away mid-session

## Polish & distribution

- [ ] Column widths adapt to terminal width instead of fixed values
- [ ] Incident age column (humanized: "3m", "2h")
- [ ] Header sparkline: instances started per refresh interval
- [x] GoReleaser config: tagged releases with darwin/linux/windows binaries (tag `vX.Y.Z` + push to trigger)
- [x] Homebrew tap (`brew install 00quasr/tap/z9s`; auto-bump via GoReleaser `brews` once TAP_GITHUB_TOKEN secret exists)
- [ ] Demo GIF in the README (vhs tape)

## Testing

- [x] httptest-based unit tests for the camunda client (auth transports: basic, OAuth caching/401-retry/expiry edges)
- [ ] httptest-based unit tests for the search/action endpoints
- [ ] teatest golden-file test for the instances view
