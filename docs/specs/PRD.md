# systemspec-credential: Product Requirements

## Problem

A deployment creates credentials faster than anyone keeps track of them:
signing keys, session keys, database passwords, OAuth clients, CI tokens,
cloud access keys, origin certificates. Each lives in a different place,
was made a different way, can do different things and has its own
expiry or rotation schedule. The knowledge of what exists, why, and when
it must change ends up spread across runbooks, console screens and
memory. Then a token expires and CI breaks, a key is never rotated, or a
leaked credential cannot be revoked quickly because nobody knows what it
can reach or what breaks when it changes.

Secret stores hold values, but each sees only its own secrets, and none
records intent: what a credential is for, how to make it, what it may
do, who uses it, and how it must be rotated.

## Users

- **Operators** who deploy and run systems and must create, rotate and
  revoke their credentials.
- **Reviewers** (security, privacy, audit) who need a complete, current
  list of credentials and what each can reach.
- **Tooling** (deploy checks, CI) that must refuse to proceed when a
  credential is expired or overdue.

## Goals

1. **One registry of every credential, without values.** Each entry
   records what it is, where it is generated and how, what it is
   entitled to do, where it is stored, who uses it and through what,
   who owns it, how it rotates, what breaks when it does, and how to
   revoke it.
2. **Defined in Go code or JSON.** A team that already writes its
   infrastructure in Go builds the registry in Go, so one loop over its
   systems and environments produces their entries. Others write JSON.
   Both validate against the same rules, and the JSON Schema is
   generated from the Go types.
3. **Lifecycle state.** An append-only event log records when each
   credential was created, rotated, verified, given an expiry or
   revoked. From it and the registry, each credential has a state: ok,
   rotation due, overdue, expiring, expired, unrecorded, planned or
   retired.
4. **A report people use.** A single, self-contained HTML file to
   search and filter by name, system, environment, kind, issuer, store,
   consumer, owner, rotation state and expiry, with every attribute and
   the history of each credential. A Markdown reference for
   documentation sites. JSON for other tools.
5. **A gate.** A command that exits non-zero when any credential is in a
   chosen state, such as expired or overdue, for deploy scripts and CI.
6. **Embeddable.** The command line is a library: a host tool mounts the
   same commands over its own Go-defined registry.

## Non-Goals

- **Storing or reading secret values.** systemspec-credential never holds a
  value. Stores such as a cloud parameter store, a secrets manager or a
  password manager do.
- **Issuing credentials** at their issuer (GitHub, a cloud provider) in
  the first releases. Generation is documented, and later automated for
  random values only.
- **A hosted service.** The first releases are a library and a command
  line over files kept in version control.

## Requirements

| # | Requirement | Priority |
|---|---|---|
| R1 | A credential entry with the attributes in goal 1 | Must |
| R2 | Validation: unique IDs, required fields, known values for closed sets, references to other entries resolve | Must |
| R3 | Load and save a registry and an event log as JSON with stable formatting | Must |
| R4 | JSON Schemas generated from the Go types | Must |
| R5 | State from the registry, the events and a point in time, with a warning window | Must |
| R6 | Self-contained HTML report with search, filters, sorting and details | Must |
| R7 | Markdown reference that depends only on the registry, so it can be checked for currency | Must |
| R8 | A command line: validate, status (with a failing gate), report, record an event, export, schema | Must |
| R9 | The command line as a mountable Cobra command over any registry source | Must |
| R10 | Observe stores to find missing, unlisted and expired credentials | Later |
| R11 | Generate random values straight into a store | Later |
| R12 | Findings as OCSF events | Later |

## Success Measures

- Every credential a deployment needs is in the registry before it is
  created, and the report is the first place an operator looks.
- No credential expires or passes its rotation date unnoticed: the gate
  catches it first.
- A leaked credential's reach, consumers and revocation steps are known
  from one entry.
