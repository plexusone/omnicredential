# OmniCredential: Roadmap

## Phase 1: Registry

- [x] **RMI-OMNICREDENTIAL-001** Credential types: the entry, its parts, well-known and closed values
- [x] **RMI-OMNICREDENTIAL-002** Validation of a registry, reporting every problem with its entry
- [x] **RMI-OMNICREDENTIAL-003** Event log: types, validation, sorted append, records
- [x] **RMI-OMNICREDENTIAL-004** State: assessment of each credential at a time, with a warning window
- [x] **RMI-OMNICREDENTIAL-005** JSON load and save; JSON Schemas generated from the types

## Phase 2: Reports and Command Line

- [x] **RMI-OMNICREDENTIAL-006** Report rows and the JSON report
- [x] **RMI-OMNICREDENTIAL-007** Self-contained HTML report with search, filters, sorting and details
- [x] **RMI-OMNICREDENTIAL-008** Markdown reference, independent of time
- [x] **RMI-OMNICREDENTIAL-009** Mountable commands: validate, status, report, record, export, schema
- [x] **RMI-OMNICREDENTIAL-010** Standalone command over JSON files
- [x] **RMI-OMNICREDENTIAL-020** Handling: whether a person or a system generates, distributes and rotates each credential, counted in every report
- [x] **RMI-OMNICREDENTIAL-021** Reach: what a holder can get (`reaches`, with `dependsOn`), what a leak exposes in all and what exposes each credential
- [x] **RMI-OMNICREDENTIAL-022** Human instructions: install steps and consoles per store, accounts with protections and recovery, and a by-place checklist
- [x] **RMI-OMNICREDENTIAL-023** Management plan: summary, launch checklist in dependency order, rotation calendar, account protections and exposure
- [x] **RMI-OMNICREDENTIAL-024** Calendar export: rotations and expiries as iCalendar events with reminders
- [x] **RMI-OMNICREDENTIAL-025** Work progress: to create, to deploy, deployed, from a deployed event, with each credential's next step in every report

## Phase 3: Observed State

- [ ] **RMI-OMNICREDENTIAL-011** Observer interface: what a store reports about a path without its value
- [ ] **RMI-OMNICREDENTIAL-012** Checks: missing, unlisted, and expiry from the issuer
- [ ] **RMI-OMNICREDENTIAL-013** Observer for a cloud parameter store
- [ ] **RMI-OMNICREDENTIAL-014** Observer for CI secrets and token expiry
- [ ] **RMI-OMNICREDENTIAL-015** Entitlement checks where the issuer reports them

## Phase 4: Lifecycle Automation

- [ ] **RMI-OMNICREDENTIAL-016** Generate random values straight into a store
- [ ] **RMI-OMNICREDENTIAL-017** Rotation reminders from the due and expiring states
- [ ] **RMI-OMNICREDENTIAL-018** Findings as OCSF events
- [ ] **RMI-OMNICREDENTIAL-019** MCP server over the registry and the log
