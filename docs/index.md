# systemspec-credential

A registry of credentials **without their values**: what each one is for,
where and how it is generated, what it is entitled to do, where it is
stored, who uses it, who owns it, and how it rotates. An event log records
when each was created, rotated, deployed, verified, given an expiry or
revoked. From the two, `credspec` works out where every credential stands,
writes reports for people and documentation sites, and fails a deploy
script when something is expired or overdue.

It is both a management tool and documentation: the registry says what
should exist, the log says what happened, and the reports show what to do
next.

## What It Answers

- **What credentials do we have?** Every secret, key, token, certificate
  and account, per system and environment, in one validated file.
- **Who does what?** Whether a person or a system generates, distributes
  and rotates each one, and how many need you by hand.
- **What do I do next?** Each credential is `to-create`, `to-record`,
  `to-deploy` or `deployed`, with its next step: the console or command
  that makes it and how to install it in each store.
- **What is due?** A lifecycle state from the rotation policy and the
  events: `ok`, `due`, `overdue`, `expiring`, `expired` and more, and a
  calendar of every rotation and expiry.
- **What does a leak expose?** `reaches` and `dependsOn` link credentials,
  so each shows everything its holder can get and which others expose it.

## No Values

No type has a field for a value, and no command reads one. Values stay in
their stores: a cloud parameter store, a secrets manager, CI secrets or a
password manager. The registry records where they are.

!!! warning "Keep reports private"
    Reports hold no values, but they list where every credential is stored
    and what it can reach, which is as sensitive as an inventory. Keep them
    with the registry, in a private repository, not on a public site.

## Where to Start

| You want to | Read |
|---|---|
| Write a first registry and run the commands | [Getting Started](guides/getting-started.md) |
| Describe a credential fully | [The Registry](guides/registry.md) |
| Record what happened and see what to do next | [Events, Work and States](guides/events-and-states.md) |
| Produce the HTML report, the plan or the calendar | [Reports](guides/reports.md) |
| Build the registry in Go or mount the commands in your tool | [Using It from Go](guides/go.md) |

## Part of the SystemSpec Family

systemspec-credential is one of a family of Go-first specifications, each
describing one aspect of a system as validated JSON with generated
schemas: architecture, cryptography, design systems, API style, pricing
and credentials. Each stands alone, and they refer to one another by ID
rather than by importing each other.

## Install

```bash
go install github.com/plexusone/systemspec-credential/cmd/credspec@latest
```

The core packages, `credspec` and `report`, depend only on the standard
library and oscompat, which has no dependencies of its own.
