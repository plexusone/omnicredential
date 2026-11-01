# Getting Started

This guide installs `credspec`, runs it over the example registry, and then
walks through the loop you repeat for real credentials: declare, create,
record, deploy, report.

## Install

```bash
go install github.com/plexusone/systemspec-credential/cmd/credspec@latest
```

## Try the Example

The repository's `examples/basic` holds a registry of six credentials for
an imaginary `example-app`, and an event log:

```bash
cd examples/basic
credspec validate
credspec status --as-of 2026-06-01
```

`credspec` reads `credentials.json` and `events.json` from the current
directory unless you pass `--registry` and `--events`. `--as-of` fixes the
date so the output matches this page:

```text
WORK       STATE       CREDENTIAL                    OWNER             BY HAND                     REASON
deployed   overdue     example-app/prod/DB_PASSWORD  platform team     generate,rotate             rotation was due 2026-03-01
to-record  unrecorded  example-org/github-owner      repository admin  generate,distribute,rotate  active, but no created or rotated event
2 of 6 credentials listed, as of 2026-06-01
Error: 1 credential(s) expired or overdue
```

`status` lists what needs attention and exits non-zero when anything is
`expired` or `overdue`, which makes it a gate for deploy scripts and CI.

## Write a Registry

A registry is a JSON file listing credentials. One entry is one secret in
one place for one environment. This is the smallest useful entry:

```json
{
  "$schema": "https://github.com/plexusone/systemspec-credential/schema/registry.schema.json",
  "formatVersion": "1",
  "credentials": [
    {
      "id": "example-app/prod/SESSION_KEY",
      "name": "SESSION_KEY",
      "system": "example-app",
      "environment": "prod",
      "kind": "random-secret",
      "status": "planned",
      "owner": "platform team",
      "issuer": { "type": "self" },
      "generation": { "method": "random", "command": "openssl rand -hex 32" },
      "stores": [
        { "type": "aws-ssm", "role": "primary", "path": "/example/prod/SESSION_KEY" }
      ],
      "rotation": { "policy": "scheduled", "intervalDays": 180 },
      "handling": { "generate": "human", "distribute": "system", "rotate": "human" }
    }
  ]
}
```

The `$schema` line gives editors completion and checking. `credspec schema
registry` writes the same schema to a file. [The Registry](registry.md)
describes every field, including the ones that make the reports useful:
where to create the value, how to install it in each store, what it is
entitled to do and what its leak exposes.

## The Loop

1. **Declare** each credential before it exists, with `status: planned`.
   `credspec status --progress to-create` lists what is still to make.
2. **Create** it, following its `generation` command or console steps,
   straight into its primary store.
3. **Record** that it exists:

    ```bash
    credspec record example-app/prod/SESSION_KEY created --by ops
    ```

    Then change its `status` to `active`.

4. **Deploy** it where its consumers read it, and record that too. A
   pattern records many at once:

    ```bash
    credspec record 'example-app/prod/*' deployed --by deploy
    ```

5. **Report** and repeat. `credspec status` in CI catches what comes due;
   `credspec report` writes the HTML report, the plan and the calendar.

[Events, Work and States](events-and-states.md) explains how events turn
into work and states.

## Next

- Write the HTML report and open it: `credspec report -o credentials.html`.
  See [Reports](reports.md).
- With many systems and environments, build the registry in Go so one
  function makes each environment's entries. See [Using It from Go](go.md).
