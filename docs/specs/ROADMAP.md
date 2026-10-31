# OmniCredential: Roadmap

## Phase 1: Registry

- [ ] **RMI-OMNICREDENTIAL-001** Credential types: the entry, its parts, well-known and closed values
- [ ] **RMI-OMNICREDENTIAL-002** Validation of a registry, reporting every problem with its entry
- [ ] **RMI-OMNICREDENTIAL-003** Event log: types, validation, sorted append, records
- [ ] **RMI-OMNICREDENTIAL-004** State: assessment of each credential at a time, with a warning window
- [ ] **RMI-OMNICREDENTIAL-005** JSON load and save; JSON Schemas generated from the types

## Phase 2: Reports and Command Line

- [ ] **RMI-OMNICREDENTIAL-006** Report rows and the JSON report
- [ ] **RMI-OMNICREDENTIAL-007** Self-contained HTML report with search, filters, sorting and details
- [ ] **RMI-OMNICREDENTIAL-008** Markdown reference, independent of time
- [ ] **RMI-OMNICREDENTIAL-009** Mountable commands: validate, status, report, record, export, schema
- [ ] **RMI-OMNICREDENTIAL-010** Standalone command over JSON files

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
