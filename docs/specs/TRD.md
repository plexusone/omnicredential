# OmniCredential: Technical Requirements

## Packages

| Package | Purpose | Dependencies |
|---|---|---|
| `omnicredential` | Types, validation, event log, state | Standard library only |
| `report` | HTML, Markdown and JSON reports | Standard library only |
| `schema` | JSON Schemas generated from the types, embedded | `github.com/invopop/jsonschema` |
| `cli` | The commands as a mountable Cobra command | `github.com/spf13/cobra` |
| `cmd/omnicredential` | The standalone command over JSON files | `cli` |

Importing the root package or `report` adds no third-party dependency.

## Data Model

One `Credential` is one secret in one place for one environment: the
staging and production copies of a key are two entries. Go code builds
both from one function.

| Field | Type | Notes |
|---|---|---|
| `id` | string | Stable and unique, such as `example-app/prod/SESSION_KEY`. Letters, digits and `._/:@-` |
| `name` | string | Display name, often the variable or parameter name |
| `description` | string | What it is for |
| `system` | string | The system that owns it |
| `environment` | string | `prod`, `staging` and so on; empty for one shared by every environment |
| `kind` | slug | Well-known values listed below; others allowed |
| `status` | enum | `planned`, `active`, `retired` |
| `sensitivity` | enum | `low`, `medium`, `high`, `critical` |
| `owner` | string | A person or role who creates and rotates it |
| `issuer` | object | `type` (slug), `account`, `console` (URL) |
| `generation` | object | `method` (enum), `spec`, `where`, `command`, `steps` |
| `entitlements` | list | `resource`, `access`, `note` |
| `stores` | list | `type` (slug), `role` (enum), `account`, `path`, `console` (URL), `install` (how the value gets there), `note` |
| `consumers` | list | `system`, `component`, `via`, `note` |
| `rotation` | object | `policy` (enum), `intervalDays`, `procedure`, `impact` |
| `handling` | object | `generate`, `distribute`, `rotate` (each `human` or `system`), `automation` |
| `revocation` | string | How to revoke it quickly |
| `exposure` | string | What a holder of the value can do |
| `dependsOn` | list | IDs this one is derived from, such as a DSN from a password |
| `reaches` | list | IDs a holder of this one can read, use or replace |
| `protections` | list | What guards it besides secrecy, such as MFA |
| `recovery` | string | How to regain access when it is lost, or that it cannot be |
| `tags` | list | Free labels |
| `links` | list | `title`, `url` |

Well-known values (open sets, validated as lowercase slugs):

- **kind:** `random-secret`, `password`, `private-key`, `certificate`,
  `connection-string`, `oauth-client`, `api-token`,
  `personal-access-token`, `access-key`, `encryption-key`, `ssh-key`,
  `federated`, `account` (a person's login that controls other
  credentials).
- **issuer type:** `self`, `aws`, `cloudflare`, `github`, `google-cloud`,
  `postgres`, `stripe`, `registrar`.
- **store type:** `aws-ssm`, `aws-secrets-manager`, `aws-kms`,
  `github-actions`, `password-manager`, `pulumi-config`, `host-file`,
  `local-file`, `none`.

Closed sets:

- **generation method:** `random`, `keypair`, `issued`, `derived`,
  `federated`.
- **store role:** `primary` (the source of truth), `backup` (an offline
  copy), `rendered` (written from the primary at deploy time).
- **rotation policy:** `scheduled` (every `intervalDays`), `on-expiry`
  (before the issuer's expiry), `on-demand` (only on compromise or need).
- **actor**, for each `handling` stage: `human` (a person, by hand) or
  `system`. `generate` makes the value, `distribute` gets it from its
  primary store to its consumers, `rotate` replaces it. `automation`
  names the systems doing the `system` stages.

## Validation

- `id`, `name`, `system`, `kind`, `status`, `owner`, `generation.method`,
  `rotation.policy` and each `handling` stage are required; IDs are unique.
- Closed sets hold only their values; open sets are slugs.
- At least one store with role `primary`, unless the method is
  `federated`.
- `scheduled` requires `intervalDays` > 0; other policies forbid it.
- `dependsOn` and `reaches` name existing IDs and never the entry itself.
- Link and console URLs are absolute `https` URLs.
- All problems are reported together, each with the entry's ID.

## Event Log

An append-only JSON list of events: `credential` (an ID), `type`, `at`,
optional `expires`, `by` and `note`.

| Type | Meaning | `expires` |
|---|---|---|
| `created` | The first value exists | Optional |
| `rotated` | A new value replaced the old one | Optional |
| `expires` | The issuer set or changed the expiry | Required |
| `verified` | Someone confirmed the value is present and works | Not allowed |
| `revoked` | The value no longer works | Not allowed |

Events name known IDs. `Append` keeps the log sorted by time, then
credential, then type, so saving is stable. A credential's record is the
latest of each: created, rotated (or created), expiry, verified, revoked.
A `created` or `rotated` event without `expires` clears the expiry.

## Reach

`Exposes(id)` follows `dependsOn` and `reaches` from a credential, and
their links in turn, to every credential a holder can get: what a leak
exposes in all. `ExposedBy(id)` follows them backwards: whose leak
exposes this one. Cycles are allowed.

## State

`Assess(credential, record, now, warn)` gives one state, the reason and
the next date that matters.

| Order | State | When |
|---|---|---|
| 1 | `retired` | Status `retired`, or a `revoked` event |
| 2 | `planned` | Status `planned` |
| 3 | `unrecorded` | Active, with no `created` or `rotated` event |
| 4 | `expired` | The expiry has passed |
| 5 | `overdue` | `scheduled`, and the last rotation plus the interval has passed |
| 6 | `expiring` | The expiry is within `warn` |
| 7 | `due` | `scheduled`, and the rotation date is within `warn` |
| 8 | `unknown-expiry` | `on-expiry`, with no expiry recorded |
| 9 | `ok` | Otherwise |

The default warning window is 30 days.

## Reports

All three are built from the same rows: each credential, its record,
its assessment and its events. Each counts the credentials that need a
person for at least one stage, those handled entirely by systems, and
each stage's human and system counts, the scheduled rotations a person
does in a year (the sum of 365 / interval), and how many a person rotates
at the issuer's expiry. Each row lists what it exposes and what exposes
it.

- **HTML:** one file with no external requests. It holds the rows as
  JSON in a `<script type="application/json">` element, written through
  `html/template`, and a short script draws the table. It offers a text
  search over ID, name, description and paths; filters for system,
  environment, kind, state, issuer, store, consumer, owner, sensitivity
  and tag, and by who does each stage; sortable columns; counts per state;
  the handling counts; and a detail panel per
  row with every attribute and the history. Light and dark themes
  follow the viewer's setting. The page is assessed at the start of a
  day (UTC) and records that date in a `meta` element, so a committed
  copy can be rebuilt from the same registry, events and date and
  compared byte for byte.
- **Markdown:** a "by hand, by place" checklist of the credentials a
  person makes, grouped by issuer and account, each with its store; then
  a section per system with a table and the details of each entry,
  including how to install it in each store. It uses only the registry, never the time or the events,
  so a test can check a committed copy is current.
- **Plan:** the management plan in Markdown: a summary (counts by
  status, system and environment, state and who does each stage); the
  launch checklist of what a person creates (planned, or active with
  nothing recorded), accounts first, then those of no one environment,
  then each environment in a given order and its systems in a given
  order, each credential after those it is built from, with
  where to create it, the command, how to install it in each store and
  the command that records it; the rotation calendar; the accounts'
  protections, ticked by `verified` events; and the credentials whose
  leak exposes the most, with what cannot be recovered and its backup.
- **Calendar:** the rotation calendar as an iCalendar (RFC 5545) file:
  an all-day event per scheduled rotation (repeating at its interval) and
  per expiry, with a reminder before it, and the procedure, impact,
  console and record command in its description. Late items go on the
  report's date. Event IDs are stable, so a newer file updates the same
  events.
- **JSON:** the rows with their assessments, for other tools.

The HTML page, the plan and the calendar depend on the time. Each
records its date (a `meta` element, a comment, an `X-` property), which
`report.AsOf` reads, so a committed copy can be rebuilt and compared.

## Command Line

```
omnicredential validate
omnicredential status   [--fail-on expired,overdue] [filters]
omnicredential report   --format html|markdown|plan|ics|json [-o file] [--as-of date]
                        [--months 12] [--environment-order prod,staging] [--system-order a,b] [--remind-days n]
omnicredential record   <id> <created|rotated|expires|verified|revoked> [--at] [--expires] [--by] [--note]
omnicredential export   [-o file]
omnicredential schema   registry|events [-o file]
```

The standalone command reads `--registry` (JSON) and `--events`. A host
mounts the same tree with `cli.New(cli.Options{Use, Registry, EventsPath})`,
where `Registry` is a function that builds the registry, for example from
Go code. `export` writes a Go-built registry as JSON.

## Security

- No type has a field for a value, and no command reads one.
- The HTML report escapes all registry text; the JSON element is
  written in a script data context, so `</script>` in a description
  cannot end it.
- Reports name stores and paths. They are as sensitive as an inventory,
  and are kept with the registry, not published.

## Testing

Unit tests for validation, the event log, every state and boundary,
each report (including escaping), the schema's currency and the
commands. No network.
