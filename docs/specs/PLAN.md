# systemspec-credential: Plan

## Approach

Build the library first, then the reports, then the command line over
them, so a host tool can use each layer on its own. Each phase ends
with a usable release.

| Phase | Delivers | Release |
|---|---|---|
| 1. Registry | Types, validation, event log, state, JSON, schemas | v0.1.0 with phase 2 |
| 2. Reports and command line | HTML, Markdown and JSON reports; the mountable commands; the standalone command | v0.1.0 |
| 3. Observed state | An observer interface and checks for missing, unlisted and expired credentials; observers for a cloud parameter store and CI secrets | v0.2.0 |
| 4. Lifecycle automation | Random values generated into a store; rotation reminders; findings as OCSF events; an MCP server | v0.3.0 |

## Design Decisions

- **No values, ever.** Keeping values out of the model keeps the
  registry safe to version and review, and leaves storage to the tools
  built for it.
- **One entry per secret, place and environment.** Flat entries are
  simple to filter and to assess. Go code removes the repetition.
- **Events, not dates in the registry.** Definitions change when intent
  changes; events change when an operator acts. Keeping them apart
  keeps the registry reviewable and the history auditable.
- **Open sets for kinds, issuers and stores.** New services appear all
  the time; a slug is enough to filter on. Closed sets are kept for the
  values the code acts on.
- **The Markdown reference is independent of time,** so a committed copy
  can be tested for currency; the HTML report includes state and is
  generated on demand.

## Risks

| Risk | Mitigation |
|---|---|
| The registry drifts from the stores | Phase 3 compares them; until then the event log and `verified` events |
| Operators forget to record events | `unrecorded` is a state of its own, shown first |
| The report leaks an inventory | It holds no values; it is kept beside the registry, never published |
