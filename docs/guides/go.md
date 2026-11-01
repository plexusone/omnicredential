# Using It from Go

The registry can be a JSON file, but with several systems and environments
it is easier to build in Go: one function makes each environment's entries,
so staging and production cannot drift apart. The same validation applies
either way.

## Packages

| Package | Purpose | Dependencies |
|---|---|---|
| `credspec` | Types, validation, event log, states, progress, reach | Standard library and oscompat |
| `report` | HTML, Markdown, plan, calendar and JSON reports | Standard library and oscompat |
| `schema` | The JSON Schemas, generated from the types and embedded | invopop/jsonschema |
| `cli` | The commands as a Cobra command you can mount | Cobra |

```bash
go get github.com/plexusone/systemspec-credential
```

## Building a Registry

```go
import "github.com/plexusone/systemspec-credential/credspec"

func sessionKey(env string) credspec.Credential {
	return credspec.Credential{
		ID:          "example-app/" + env + "/SESSION_KEY",
		Name:        "SESSION_KEY",
		System:      "example-app",
		Environment: env,
		Kind:        credspec.KindRandomSecret,
		Status:      credspec.StatusActive,
		Owner:       "platform team",
		Issuer:      credspec.Issuer{Type: credspec.IssuerSelf},
		Generation:  credspec.Generation{Method: credspec.MethodRandom, Command: "openssl rand -hex 32"},
		Stores: []credspec.Store{{
			Type: credspec.StoreAWSSSM, Role: credspec.RolePrimary,
			Path: "/example/" + env + "/SESSION_KEY",
		}},
		Rotation: credspec.Rotation{Policy: credspec.PolicyScheduled, IntervalDays: 180},
		Handling: credspec.Handling{
			Generate: credspec.ActorHuman, Distribute: credspec.ActorSystem, Rotate: credspec.ActorHuman,
		},
	}
}

func Registry() (*credspec.Registry, error) {
	reg := credspec.NewRegistry()
	for _, env := range []string{"staging", "prod"} {
		reg.Add(sessionKey(env))
	}
	return reg, reg.Validate()
}
```

`Validate` returns every problem at once, each with its entry's ID. A test
that calls `Registry()` keeps the registry valid in CI.

Other entry points:

- `LoadRegistry` and `ReadRegistry` read JSON, refusing unknown fields.
- `LoadLog` reads the event log and checks it against the registry; a
  missing file is an empty log.
- `Assess` and `ProgressOf` give one credential's state and progress from
  its record (`log.Record(id)`).
- `Exposes` and `ExposedBy` follow `dependsOn` and `reaches`.

## Mounting the Commands

`cli.New` returns the whole command tree, `validate`, `status`, `report`,
`record`, `export` and `schema`, over a registry your code builds:

```go
import "github.com/plexusone/systemspec-credential/cli"

root.AddCommand(cli.New(cli.Options{
	Use:        "credentials",
	Title:      "Example credentials",
	Registry:   Registry,
	EventsPath: "credentials/events.json",
}))
```

Your tool then has `mytool credentials status`, `mytool credentials
record …` and the rest. With `Registry` set, there is no `--registry`
flag. `export` writes the Go-built registry as JSON, for other tools and
for the schema-aware editors.

## Writing Reports

```go
import "github.com/plexusone/systemspec-credential/report"

rep := report.Build("Example credentials", reg, log, time.Now(), credspec.DefaultWarn)
err := rep.HTML(w)
err = rep.Plan(w, reg, report.PlanOptions{EnvironmentOrder: []string{"prod", "staging"}})
err = rep.Calendar(w, report.CalendarOptions{})
err = report.Markdown(w, "Example credentials", reg)
```

## Checking Committed Reports

Commit the reports next to the registry and let a test fail when one is
out of date. The page records its own date, so the test rebuilds it for
that date and compares:

```go
func TestReportCurrent(t *testing.T) {
	reg, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	log, err := credspec.LoadLog("credentials/events.json", reg)
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile("credentials/credentials.html")
	if err != nil {
		t.Fatal(err)
	}
	asOf, err := report.AsOf(page)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := report.Build("Example credentials", reg, log, asOf, credspec.DefaultWarn).HTML(&b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), page) {
		t.Error("credentials.html is out of date: regenerate the reports")
	}
}
```

The Markdown reference needs no date: rebuild it from the registry alone
and compare.
