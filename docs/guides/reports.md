# Reports

`credspec report` writes five formats from the same rows: each credential,
its record, its state, its progress and next step, its history, and what
its leak exposes.

```bash
credspec report --format html     -o credentials.html
credspec report --format markdown -o reference.md
credspec report --format plan     -o plan.md
credspec report --format ics      -o credentials.ics
credspec report --format json     -o report.json
```

!!! warning "Keep reports private"
    No report holds a value, but each lists where every credential is
    stored and what it can reach. Keep them with the registry, in a private
    repository, not on a public site.

## HTML Report

One self-contained file that makes no network requests. It opens sorted
by work progress, so what you have to do comes first.

- **Search** over IDs, names, descriptions and paths.
- **Filters** for work, system, environment, kind, state, issuer, store,
  consumer, owner, sensitivity, rotation, who generates, distributes and
  rotates, and tag.
- **Chips** count each kind of work within the other filters; the "you
  create" chip shows only what a person makes by hand.
- **Details** for every credential: each attribute, its next step, what it
  exposes and what exposes it, and its history.
- **CSV** download of the rows shown.

Filters live in the URL's fragment, so a link opens the same view. For
example, `credentials.html#progress=to-create&generate=human` shows what
you create by hand, and `#tag=launch` the credentials tagged `launch`.

## Markdown Reference

A page for a documentation site: a "by hand, by place" checklist of what a
person creates, grouped by issuer and account, then a section per system
with every credential's details and how to install it in each store.

It depends only on the registry, never on the time or the events, so a
test can regenerate it and fail when the committed copy is out of date.

## Management Plan

The plan, in Markdown, answers "what do I do, and when":

1. **Summary:** counts by status, system and environment, state, work and
   who does each stage, and the rotations a person does in a year.
2. **Launch checklist:** what a person creates, in the order to do it:
   accounts first, then shared credentials, then each environment and its
   systems, each credential after those it is built from. Each item has
   its console, command, where to install it and the command that records
   it.
3. **To record** and **to deploy:** credentials waiting for an event.
4. **Rotation calendar:** every rotation and expiry in the next 12 months.
5. **Account protections**, ticked by `verified` events.
6. **Exposure:** the credentials whose leak exposes the most, and what
   cannot be recovered, with its backup.

Order the checklist with `--environment-order prod,staging` and
`--system-order shared,api,web`, and change the calendar's horizon with
`--months`.

## Calendar

An iCalendar (RFC 5545) file with an all-day event for each scheduled
rotation, repeating at its interval, and each expiry. Each event has a
reminder before it (`--remind-days`, by default the warning window) and
the procedure, impact, console and record command in its description.
Late items are placed on the report's date.

Event IDs are stable, so importing a newer file updates the same events
rather than adding copies.

## JSON

The rows with their records, states and progress, for other tools.

## Committing Reports

The HTML report, the plan and the calendar depend on the date, so each
records the date it was assessed: a `meta` element, a comment or an
`X-CREDSPEC-AS-OF` property. The time is taken at the start of the day
(UTC).

To check a committed copy is current, read its date with `report.AsOf`,
rebuild it with `--as-of` that date, and compare. [Using It from
Go](go.md#checking-committed-reports) shows the test.
