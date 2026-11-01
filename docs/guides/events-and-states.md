# Events, Work and States

The registry says what should exist. The event log says what happened.
From the two, each credential gets a **work progress** (what to do next)
and a **lifecycle state** (whether it is due).

## The Event Log

`events.json` is an append-only list. Each event names a credential, a
`type` and a time `at`, with optional `expires`, `by` and `note`.

| Type | Meaning | `expires` |
|---|---|---|
| `created` | The first value exists | Optional |
| `rotated` | A new value replaced the old one | Optional |
| `expires` | The issuer set or changed the expiry | Required |
| `deployed` | The value is in place in its `rendered` stores | Not allowed |
| `verified` | Someone confirmed the value is present and works | Not allowed |
| `revoked` | The value no longer works | Not allowed |

A `created` or `rotated` event without `expires` clears any earlier
expiry, because the new value has none.

## Recording

```bash
credspec record example-app/prod/SESSION_KEY created --by ops
credspec record example-app/ci/MODULES_TOKEN rotated --expires 2027-05-01 --by admin
credspec record 'example-app/prod/*' deployed --by deploy
credspec record example-org/github-owner verified --note "hardware key checked"
```

- `--at` records something that happened earlier, as a date or an RFC 3339
  time; the default is now.
- `*` in the ID matches any text, slashes included, so one command records
  every match.
- The log is kept sorted by time, then credential, then type, so it diffs
  cleanly in version control.

Only `credspec record` needs to touch the file; it validates each event
against the registry before writing.

## Work Progress

Progress says where the work on a credential stands. It drives the HTML
report's default order, the plan's checklists and `status --progress`.

| Progress | When | Next step |
|---|---|---|
| `to-create` | Planned, with no `created` or `rotated` event | Create it: the console or command, then install it in each store |
| `to-record` | Active, so it exists, but nothing is recorded | Record it as `created`, with `--at` the date it was made |
| `to-deploy` | It has a `rendered` store and no `deployed` event since it last changed | Deploy it, then record `deployed` |
| `deployed` | Created, and deployed since; or no `rendered` store | Nothing |
| `retired` | Status `retired`, or a `revoked` event | Nothing |

A rotation sends a credential back to `to-deploy` until the next
`deployed` event. A federated credential has no value: it counts as
created once deployed.

```bash
credspec status --progress to-create,to-record --manual
```

lists what you, rather than a system, still have to make or record.

## Lifecycle States

The state says whether a credential needs attention now. The first state
that applies wins:

| State | When |
|---|---|
| `retired` | Status `retired`, or a `revoked` event |
| `planned` | Planned, and not created yet |
| `unrecorded` | Active, with no `created` or `rotated` event |
| `expired` | The expiry has passed |
| `overdue` | Scheduled, and the last change plus the interval has passed |
| `expiring` | The expiry is within the warning window |
| `due` | Scheduled, and the rotation date is within the warning window |
| `unknown-expiry` | Rotates on expiry, with no expiry recorded |
| `ok` | Otherwise |

The warning window is 30 days; `--warn-days` changes it. A planned
credential with a `created` event is assessed like an active one, so you
can create credentials ahead of a launch.

## Using `status` as a Gate

`status` exits non-zero when any credential is in a `--fail-on` state,
`expired,overdue` by default:

```bash
credspec status                          # fail on expired or overdue
credspec status --fail-on expired,overdue,expiring
credspec status --fail-on ''             # list only, never fail
```

Filter with `--system`, `--env`, `--state` and `--progress`, or pass
`--all` to list every credential.
