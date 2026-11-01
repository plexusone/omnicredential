# The Registry

The registry lists every credential a set of systems needs, without values.
It validates against rules that are the same whether you write JSON or
build it in Go, and its JSON Schema is generated from the Go types.

## One Entry per Secret, Place and Environment

The staging and production copies of a key are two entries, because they
are created, stored and rotated separately. IDs are stable and unique,
conventionally `system/environment/NAME`, using letters, digits and
`._/:@-`.

## Fields

### Identity

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Stable, unique ID |
| `name` | yes | Display name, often the variable or parameter name |
| `description` | | What it is for |
| `system` | yes | The system that owns it |
| `environment` | | `prod`, `staging` and so on; empty for one shared by every environment |
| `kind` | yes | What it is; see [Kinds](#kinds) |
| `status` | yes | `planned`, `active` or `retired` |
| `sensitivity` | | `low`, `medium`, `high` or `critical` |
| `owner` | yes | The person or role who creates and rotates it |
| `tags`, `links` | | Free labels, and `title` plus `url` links |

### Where It Comes From

`issuer` is who issues the value: `type` (such as `self`, `aws`, `github`),
`account`, and `console`, the URL where a person manages it.

`generation` is how the value is made:

| Field | Meaning |
|---|---|
| `method` | `random`, `keypair`, `issued` (by the issuer), `derived` (built from others, listed in `dependsOn`) or `federated` (no stored value) |
| `spec` | The shape, such as "32 random bytes, hex" |
| `where` | Where it is made, such as "the operator's machine, straight into the parameter store" |
| `command` | The command that makes it |
| `steps` | Console steps, in order, when there is no command |

### Where It Lives and Who Uses It

`stores` lists every place the value is kept. Each has a `type` (such as
`aws-ssm`, `github-actions`, `password-manager`, `host-file`), a `path`,
an `account`, a `console` and a `role`:

| Role | Meaning |
|---|---|
| `primary` | The source of truth. Every credential needs one, unless federated |
| `backup` | An offline copy, such as in a password manager |
| `rendered` | Written from the primary at deploy time, where consumers read it |

`install` says, in a sentence or a command, how the value gets into that
store. The reports put it in each credential's next step, so write it for
the person who will do it.

`consumers` lists who reads it: `system`, `component`, and `via`, such as
`env SESSION_KEY`.

### What It Can Do

| Field | Meaning |
|---|---|
| `entitlements` | What the issuer grants it: `resource`, `access`, `note`, such as a token's scopes |
| `exposure` | What a holder of the value can do, in a sentence |
| `revocation` | How to revoke it quickly |
| `dependsOn` | IDs it is built from, such as a connection string from a password |
| `reaches` | IDs a holder of this one can read, use or replace, such as every parameter an AWS identity can read |

`dependsOn` and `reaches` together let each report show what a leak
exposes in all, following links through other credentials, and which
credentials expose this one.

### Rotation and Handling

`rotation` says when the value is replaced:

| Policy | Meaning |
|---|---|
| `scheduled` | Every `intervalDays` |
| `on-expiry` | Before the issuer's expiry, recorded with an `expires` event |
| `on-demand` | Only on compromise or need |

`procedure` and `impact` describe how a rotation is done and what it
interrupts.

`handling` says who does each stage, `human` or `system`:

| Stage | Meaning |
|---|---|
| `generate` | Makes the value |
| `distribute` | Gets it from its primary store to its consumers |
| `rotate` | Replaces it |

`automation` names the systems doing the `system` stages. The reports
count how many credentials need a person for at least one stage, how many
are fully automatic, and how many rotations a person does in a year.

### Accounts

A person's login that controls other credentials, such as the owner of a
code-hosting organization or a cloud root user, is an entry of kind
`account`. Give it `protections` (such as "hardware security key",
"MFA") and `recovery` (how to regain access, or that it cannot be), and
list in `reaches` what it controls. The plan lists every account's
protections, and a `verified` event ticks them off.

## Kinds

`kind`, `issuer.type` and `stores[].type` are open sets: any lowercase
slug is valid, and these are the well-known values the reports and the
schema suggest.

- **kind:** `random-secret`, `password`, `private-key`, `certificate`,
  `connection-string`, `oauth-client`, `api-token`,
  `personal-access-token`, `access-key`, `encryption-key`, `ssh-key`,
  `federated`, `account`.
- **issuer type:** `self`, `aws`, `cloudflare`, `github`, `google-cloud`,
  `postgres`, `stripe`, `registrar`.
- **store type:** `aws-ssm`, `aws-secrets-manager`, `aws-kms`,
  `github-actions`, `password-manager`, `pulumi-config`, `host-file`,
  `local-file`, `none`.

## Validation

`credspec validate` reports every problem at once, each with its entry's
ID:

- required fields are present and IDs are unique;
- closed sets hold only their values, and open sets are slugs;
- every credential has a `primary` store, unless federated;
- `scheduled` has `intervalDays` above zero, and other policies have none;
- `dependsOn` and `reaches` name existing IDs, never the entry itself;
- link and console URLs are absolute `https` URLs.

A registry file refuses unknown fields, so a misspelled field is an error
rather than silently ignored.
