# OmniCredential

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/plexusone/omnicredential/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/plexusone/omnicredential/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/plexusone/omnicredential/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/plexusone/omnicredential/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/plexusone/omnicredential/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/plexusone/omnicredential/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/plexusone/omnicredential
 [docs-godoc-url]: https://pkg.go.dev/github.com/plexusone/omnicredential
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/plexusone/omnicredential/blob/main/LICENSE

A registry of credentials **without their values**: what each one is for,
where and how it is generated, what it is entitled to do, where it is
stored, who uses it, who owns it, and how it rotates. An event log records
when each was created, rotated, verified, given an expiry or revoked, and
OmniCredential turns both into a lifecycle state, reports and a gate for
deploy scripts.

It is both a management tool and documentation: the registry says what
should exist, the log says what happened, and the reports show where every
credential stands.

## Features

- **Registry in Go or JSON.** Build it in Go, so one loop makes each
  environment's entries, or write JSON. Both validate against the same
  rules; the JSON Schemas are generated from the Go types.
- **Who does what.** Each credential says whether a person or a system
  generates, distributes and rotates it, and the reports count how many
  need you by hand and how many are fully automatic.
- **What a leak exposes.** `reaches` and `dependsOn` link credentials, so
  each one shows everything a holder can get, directly or through others,
  and which credentials expose it.
- **Instructions for people.** Each store says how the value is installed
  there and where its console is; accounts record their protections and
  recovery; the reference lists what to create by hand, grouped by place.
- **Lifecycle state.** Each credential is `ok`, `due`, `overdue`,
  `expiring`, `expired`, `unknown-expiry`, `unrecorded`, `planned` or
  `retired`, from its rotation policy and its events.
- **HTML report.** One self-contained file: search, filter by system,
  environment, kind, state, issuer, store, consumer, owner, sensitivity,
  rotation and tag, sort, open any credential's details and history,
  share a filtered view by URL, and download CSV.
- **Management plan** in Markdown: a summary, the launch checklist of
  what you create by hand (in the order to do it, with the console, the
  command and where to install it), the rotation calendar for the next
  year, the accounts' protections, and what each leak exposes.
- **Calendar export.** An iCalendar file with every rotation and expiry
  and a reminder before each; import it once and your calendar tells you.
- **Markdown reference** for documentation sites. It depends only on the
  registry, so a test can check a committed copy is current.
- **A gate.** `status` exits non-zero when any credential is expired or
  overdue, or in the states you choose.
- **Embeddable.** The command line is a Cobra command a host tool mounts
  over its own registry.
- **No values.** No type has a field for one, and no command reads one.

## Install

```bash
go install github.com/plexusone/omnicredential/cmd/omnicredential@latest
```

## Use

```bash
omnicredential validate --registry credentials.json --events events.json
omnicredential status   --registry credentials.json --events events.json
omnicredential record   example-app/prod/SESSION_KEY rotated --by ops --registry credentials.json --events events.json
omnicredential report   --format html -o credentials.html --registry credentials.json --events events.json
```

| Command | What it does |
|---|---|
| `validate` | Checks the registry and the event log |
| `status` | Lists credentials needing attention; fails on `--fail-on` states (default `expired,overdue`); `--manual` for only those with a step by hand |
| `report` | Writes `--format html`, `markdown`, `plan`, `ics` or `json` |
| `record <id> <type>` | Adds a `created`, `rotated`, `expires`, `verified` or `revoked` event |
| `export` | Writes the registry as JSON |
| `schema <registry\|events>` | Writes a JSON Schema |

See [`examples/basic`](examples/basic) for a registry and an event log.

## In Go

```go
reg := omnicredential.NewRegistry()
for _, env := range []string{"staging", "prod"} {
	reg.Add(omnicredential.Credential{
		ID:          "example-app/" + env + "/SESSION_KEY",
		Name:        "SESSION_KEY",
		System:      "example-app",
		Environment: env,
		Kind:        omnicredential.KindRandomSecret,
		Status:      omnicredential.StatusActive,
		Owner:       "platform team",
		Issuer:      omnicredential.Issuer{Type: omnicredential.IssuerSelf},
		Generation:  omnicredential.Generation{Method: omnicredential.MethodRandom, Command: "openssl rand -hex 32"},
		Stores:      []omnicredential.Store{{Type: omnicredential.StoreAWSSSM, Role: omnicredential.RolePrimary, Path: "/example/" + env + "/SESSION_KEY"}},
		Rotation:    omnicredential.Rotation{Policy: omnicredential.PolicyScheduled, IntervalDays: 180},
		Handling:    omnicredential.Handling{Generate: omnicredential.ActorHuman, Distribute: omnicredential.ActorSystem, Rotate: omnicredential.ActorHuman},
	})
}
```

Mount the commands in your own tool, over that registry:

```go
root.AddCommand(cli.New(cli.Options{
	Use:        "credentials",
	Registry:   func() (*omnicredential.Registry, error) { return reg, nil },
	EventsPath: "credentials/events.json",
}))
```

## Packages

| Package | Purpose |
|---|---|
| `omnicredential` | Types, validation, event log, state (standard library only) |
| `report` | HTML, Markdown and JSON reports (standard library only) |
| `schema` | JSON Schemas generated from the types |
| `cli` | The commands as a mountable Cobra command |
| `cmd/omnicredential` | The standalone command |

## Keep Reports Private

Reports hold no values, but they list where every credential is stored and
what it can reach. Keep them with the registry, not on a public site.

## Specs

[PRD](docs/specs/PRD.md) · [TRD](docs/specs/TRD.md) · [Plan](docs/specs/PLAN.md) · [Roadmap](docs/specs/ROADMAP.md)

## License

MIT
