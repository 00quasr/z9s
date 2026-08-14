# Backlog

Groomed, ticket-sized units — each one is a single evening's contribution.
Pick the top unchecked item; if it feels too big, split it first and commit the split.
Once the repo is on GitHub, migrate these to issues.

## Drill-down (the core loop)

- [ ] `enter` on an instance opens a detail view: element instance tree (`POST /v2/element-instances/search` filtered by `processInstanceKey`)
- [ ] Detail view: show instance variables (`POST /v2/variables/search`)
- [ ] `enter` on an incident jumps to its process instance detail
- [ ] `esc` navigates back from detail to the list

## Actions (what Operate makes you click for)

- [ ] `ctrl+r` on an incident: resolve it (`POST /v2/jobs/{jobKey}` update retries, then `POST /v2/incidents/{incidentKey}/resolution`)
- [ ] `ctrl+k` on an instance: cancel it, with a y/n confirm prompt
- [ ] `s` on a definition: start an instance (empty variables first; variable editor later)

## Filtering & search

- [ ] `/` filter: fuzzy-match rows in the current view client-side
- [ ] Server-side filter by state (ACTIVE / COMPLETED / TERMINATED) via the search `filter` object
- [ ] Sort instances by start date descending (search `sort` param)
- [ ] Cursor pagination: load more rows on scroll past the last row (`page.endCursor` → `after`)

## Connectivity & config

- [ ] Config file `~/.config/z9s/config.yml` with named clusters (like kubeconfig contexts)
- [ ] Bearer-token auth for SaaS / Self-Managed with identity enabled
- [ ] Graceful reconnect banner when the cluster goes away mid-session

## Polish & distribution

- [ ] Column widths adapt to terminal width instead of fixed values
- [ ] Incident age column (humanized: "3m", "2h")
- [ ] Header sparkline: instances started per refresh interval
- [ ] GoReleaser config: tagged releases with darwin/linux/windows binaries
- [ ] Homebrew tap
- [ ] Demo GIF in the README (vhs tape)

## Testing

- [ ] httptest-based unit tests for the camunda client (topology + three searches)
- [ ] teatest golden-file test for the instances view
