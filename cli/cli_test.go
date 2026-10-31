package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/omnicredential"
)

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func registry() *omnicredential.Registry {
	c := func(id string, days int) omnicredential.Credential {
		return omnicredential.Credential{
			ID: id, Name: id, System: "example-app", Environment: "prod", Kind: omnicredential.KindRandomSecret,
			Status: omnicredential.StatusActive, Owner: "ops", Issuer: omnicredential.Issuer{Type: omnicredential.IssuerSelf},
			Generation: omnicredential.Generation{Method: omnicredential.MethodRandom},
			Stores:     []omnicredential.Store{{Type: omnicredential.StoreAWSSSM, Role: omnicredential.RolePrimary, Path: "/p/" + id}},
			Rotation:   omnicredential.Rotation{Policy: omnicredential.PolicyScheduled, IntervalDays: days},
			Handling: omnicredential.Handling{Generate: omnicredential.ActorHuman, Distribute: omnicredential.ActorSystem,
				Rotate: omnicredential.ActorHuman},
		}
	}
	return omnicredential.NewRegistry(c("key-a", 30), c("key-b", 365))
}

// run executes the command with a Go-built registry and an events file in
// a temporary directory.
func run(t *testing.T, events string, args ...string) (string, error) {
	t.Helper()
	cmd := New(Options{Use: "credentials", Registry: func() (*omnicredential.Registry, error) { return registry(), nil },
		EventsPath: events, Now: func() time.Time { return now }})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestLifecycle(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events.json")

	out, err := run(t, events, "validate")
	if err != nil || !strings.Contains(out, "2 credentials and 0 events OK") {
		t.Fatalf("validate: %v\n%s", err, out)
	}

	// Nothing recorded: both are unrecorded, which lists but does not fail.
	out, err = run(t, events, "status")
	if err != nil || strings.Count(out, "unrecorded") != 2 {
		t.Fatalf("status: %v\n%s", err, out)
	}

	if out, err = run(t, events, "record", "key-a", "created", "--at", "2026-04-01", "--by", "ops"); err != nil {
		t.Fatalf("record: %v\n%s", err, out)
	}
	if _, err = run(t, events, "record", "key-b", "created", "--note", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, events, "record", "key-z", "created"); err == nil {
		t.Error("recorded an unknown credential")
	}
	if _, err := run(t, events, "record", "key-a", "expires"); err == nil {
		t.Error("recorded an expiry without --expires")
	}

	// key-a was due 2026-05-01: overdue fails the gate.
	out, err = run(t, events, "status")
	if err == nil || !strings.Contains(err.Error(), "1 credential(s) expired or overdue") {
		t.Fatalf("status gate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "overdue") || strings.Contains(out, "key-b") {
		t.Errorf("status listed:\n%s", out)
	}
	if out, err = run(t, events, "status", "--fail-on", "", "--all"); err != nil || !strings.Contains(out, "key-b") || !strings.Contains(out, "generate,rotate") {
		t.Errorf("status --all: %v\n%s", err, out)
	}
	if out, err = run(t, events, "status", "--fail-on", "", "--all", "--manual"); err != nil || !strings.Contains(out, "key-b") {
		t.Errorf("status --manual: %v\n%s", err, out)
	}
	if _, err = run(t, events, "status", "--as-of", "2026-04-15"); err != nil {
		t.Errorf("status before the due date failed: %v", err)
	}
	if _, err = run(t, events, "status", "--fail-on", "bogus"); err == nil {
		t.Error("an unknown state was accepted")
	}

	if _, err = run(t, events, "record", "key-a", "rotated", "--at", "2026-05-31T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if out, err = run(t, events, "status"); err != nil {
		t.Errorf("status after rotation: %v\n%s", err, out)
	}

	l, err := omnicredential.LoadLog(events, registry())
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Events) != 3 || l.Events[0].By != "ops" || !l.Events[1].At.Equal(time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("events = %+v", l.Events)
	}
}

func TestReports(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events.json")
	for _, f := range []string{"html", "markdown", "json"} {
		path := filepath.Join(dir, "report."+f)
		if out, err := run(t, events, "report", "--format", f, "-o", path, "--title", "Team"); err != nil {
			t.Fatalf("%s: %v\n%s", f, err, out)
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: a file in the test's temporary directory.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("Team")) || !bytes.Contains(data, []byte("key-a")) {
			t.Errorf("%s report:\n%s", f, data)
		}
	}
	if _, err := run(t, events, "report", "--format", "pdf"); err == nil {
		t.Error("an unknown format was accepted")
	}
	out, err := run(t, events, "export")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := omnicredential.ReadRegistry([]byte(out)); err != nil {
		t.Errorf("export is not a valid registry: %v", err)
	}
	for _, s := range []string{"registry", "events"} {
		if out, err := run(t, events, "schema", s); err != nil || !strings.Contains(out, `"$id"`) {
			t.Errorf("schema %s: %v", s, err)
		}
	}
	if _, err := run(t, events, "schema", "other"); err == nil {
		t.Error("an unknown schema was accepted")
	}
}

func TestStandaloneReadsRegistryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := registry().Save(path); err != nil {
		t.Fatal(err)
	}
	cmd := New(Options{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"validate", "--registry", path, "--events", filepath.Join(dir, "events.json")})
	if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), "2 credentials") {
		t.Fatalf("validate: %v\n%s", err, out.String())
	}
	cmd = New(Options{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate", "--registry", filepath.Join(dir, "missing.json")})
	if err := cmd.Execute(); err == nil {
		t.Error("a missing registry was accepted")
	}
}

func TestBadRegistryFromGo(t *testing.T) {
	cmd := New(Options{Registry: func() (*omnicredential.Registry, error) {
		return omnicredential.NewRegistry(omnicredential.Credential{ID: "x"}), nil
	}})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("err = %v", err)
	}
}

func TestParseTime(t *testing.T) {
	if _, err := parseTime("tomorrow"); err == nil {
		t.Error("parsed tomorrow")
	}
	got, err := parseTime("2026-01-02")
	if err != nil || !got.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date = %v, %v", got, err)
	}
}
