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
| `stores` | list | `type` (slug), `role` (enum), `account`, `path`, `note` |
| `consumers` | list | `system`, `component`, `via`, `note` |
| `rotation` | object | `policy` (enum), `intervalDays`, `procedure`, `impact` |
| `revocation` | string | How to revoke it quickly |
| `exposure` | string | What a holder of the value can do |
| `dependsOn` | list | IDs this one is derived from, such as a DSN from a password |
| `tags` | list | Free labels |
| `links` | list | `title`, `url` |

Well-known values (open sets, validated as lowercase slugs):

- **kind:** `random-secret`, `password`, `private-key`, `certificate`,
  `connection-string`, `oauth-client`, `api-token`,
  `personal-access-token`, `access-key`, `encryption-key`, `ssh-key`,
  `federated`.
- **issuer type:** `self`, `aws`, `cloudflare`, `github`, `google-cloud`,
  `postgres`, `stripe`.
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

## Validation

- `id`, `name`, `system`, `kind`, `status`, `owner`, `generation.method`
  and `rotation.policy` are required; IDs are unique.
- Closed sets hold only their values; open sets are slugs.
- At least one store with role `primary`, unless the method is
  `federated`.
- `scheduled` requires `intervalDays` > 0; other policies forbid it.
- `dependsOn` names existing IDs and never the entry itself.
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
its assessment and its events.

- **HTML:** one file with no external requests. It holds the rows as
  JSON in a `<script type="application/json">` element, written through
  `html/template`, and a short script draws the table. It offers a text
  search over ID, name, description and paths; filters for system,
  environment, kind, state, issuer, store, consumer, owner, sensitivity
  and tag; sortable columns; counts per state; and a detail panel per
  row with every attribute and the history. Light and dark themes
  follow the viewer's setting.
- **Markdown:** a section per system with a table and the details of
  each entry. It uses only the registry, never the time or the events,
  so a test can check a committed copy is current.
- **JSON:** the rows with their assessments, for other tools.

## Command Line

```
omnicredential validate
omnicredential status   [--fail-on expired,overdue] [filters]
omnicredential report   --format html|markdown|json [-o file] [--as-of date]
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
