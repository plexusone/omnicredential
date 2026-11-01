package omnicredential

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	oscfs "github.com/grokify/oscompat/fs"
)

func secret(id string) Credential {
	return Credential{
		ID:          id,
		Name:        "SESSION_KEY",
		Description: "Encrypts stored sessions.",
		System:      "example-app",
		Environment: "prod",
		Kind:        KindRandomSecret,
		Status:      StatusActive,
		Sensitivity: SensitivityHigh,
		Owner:       "operators",
		Issuer:      Issuer{Type: IssuerSelf},
		Generation:  Generation{Method: MethodRandom, Spec: "32 random bytes, hex", Command: "openssl rand -hex 32"},
		Stores:      []Store{{Type: StoreAWSSSM, Role: RolePrimary, Path: "/example/prod/SESSION_KEY"}},
		Consumers:   []Consumer{{System: "example-app", Via: "env SESSION_KEY"}},
		Rotation:    Rotation{Policy: PolicyScheduled, IntervalDays: 90},
		Handling:    Handling{Generate: ActorHuman, Distribute: ActorSystem, Rotate: ActorHuman, Automation: "deploy tool"},
	}
}

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(t time.Time) *time.Time { return &t }

func problemsOf(t *testing.T, err error) []string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v is not a *ValidationError", err)
	}
	out := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		out[i] = p.String()
	}
	return out
}

func wantProblem(t *testing.T, got []string, want string) {
	t.Helper()
	for _, g := range got {
		if strings.Contains(g, want) {
			return
		}
	}
	t.Errorf("no problem containing %q in:\n  %s", want, strings.Join(got, "\n  "))
}

func TestValidRegistry(t *testing.T) {
	pw := secret("example-app/prod/APP_PASSWORD")
	dsn := secret("example-app/prod/DSN")
	dsn.Kind = KindConnectionString
	dsn.Generation = Generation{Method: MethodDerived}
	dsn.DependsOn = []string{pw.ID}
	dsn.Rotation = Rotation{Policy: PolicyOnDemand}
	oidc := Credential{
		ID: "ci/aws-deploy-role", Name: "deploy role", System: "ci", Kind: KindFederated,
		Status: StatusActive, Owner: "operators", Issuer: Issuer{Type: IssuerAWS, Console: "https://console.aws.amazon.com/iam/"},
		Generation: Generation{Method: MethodFederated}, Rotation: Rotation{Policy: PolicyOnDemand},
		Handling: Handling{Generate: ActorSystem, Distribute: ActorSystem, Rotate: ActorSystem},
		Links:    []Link{{Title: "OIDC", URL: "https://docs.github.com/actions"}},
	}
	r := NewRegistry(pw, dsn, oidc)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	bad := Credential{ID: "x", Kind: "Not A Slug", Status: "live", Sensitivity: "extreme",
		Issuer:       Issuer{Console: "http://insecure.example"},
		Generation:   Generation{Method: MethodDerived},
		Stores:       []Store{{Type: StoreAWSSSM, Role: RoleBackup}},
		Consumers:    []Consumer{{}},
		Rotation:     Rotation{Policy: PolicyOnDemand, IntervalDays: 30},
		DependsOn:    []string{"x", "missing"},
		Links:        []Link{{URL: "ftp://x"}},
		Entitlements: []Entitlement{{Resource: "repo"}},
	}
	bad.Handling = Handling{Generate: "robot", Distribute: ActorSystem, Rotate: ActorHuman}
	r := &Registry{FormatVersion: "0", Credentials: []Credential{bad, secret("x"), {}}}
	got := problemsOf(t, r.Validate())
	for _, want := range []string{
		`formatVersion is "0"`,
		"x: duplicate id",
		"credentials[2]: id is required",
		"x: name is required",
		"x: system is required",
		"x: owner is required",
		`kind "Not A Slug" must be a lowercase slug`,
		`status "live" is not one of`,
		`sensitivity "extreme" is not one of`,
		`handling.generate "robot" is not one of`,
		"issuer.type is required",
		"issuer.console must be an absolute https URL",
		"needs a store with role primary",
		"consumers[0].system is required",
		"intervalDays is only for the scheduled policy",
		"dependsOn names itself",
		`dependsOn names unknown credential "missing"`,
		"links[0].title is required",
		"links[0].url must be an absolute https URL",
		"entitlements[0] needs a resource and an access",
		"credentials[2]: kind is required",
		"credentials[2]: handling.generate is required",
	} {
		wantProblem(t, got, want)
	}
}

func TestValidateScheduledNeedsInterval(t *testing.T) {
	c := secret("a")
	c.Rotation.IntervalDays = 0
	wantProblem(t, problemsOf(t, NewRegistry(c).Validate()), "intervalDays is required")
	c.Rotation.IntervalDays = -1
	wantProblem(t, problemsOf(t, NewRegistry(c).Validate()), "must not be negative")
}

func TestValidateDerivedNeedsDependencies(t *testing.T) {
	c := secret("a")
	c.Generation.Method = MethodDerived
	wantProblem(t, problemsOf(t, NewRegistry(c).Validate()), "lists what it depends on")
}

func TestValidateBadID(t *testing.T) {
	wantProblem(t, problemsOf(t, NewRegistry(secret("has space")).Validate()), "id must match")
}

func TestOpenSetsAcceptNewSlugs(t *testing.T) {
	c := secret("a")
	c.Kind = "webhook-secret"
	c.Issuer.Type = "acme-cloud"
	c.Stores[0].Type = "acme-vault"
	if err := NewRegistry(c).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	r := NewRegistry(secret("a"), secret("b"))
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Credentials) != 2 || got.Credentials[1].ID != "b" {
		t.Fatalf("got %+v", got.Credentials)
	}
	c, ok := got.Find("a")
	if !ok || c.Rotation.IntervalDays != 90 {
		t.Errorf("Find(a) = %+v, %v", c, ok)
	}
	if _, ok := got.Find("z"); ok {
		t.Error("Find(z) found something")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Modes are not enforced on every platform (Windows).
	if oscfs.PermissionsEnforced() && fi.Mode().Perm() != oscfs.DefaultFilePerm {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestReadRegistryRefusesUnknownFields(t *testing.T) {
	_, err := ReadRegistry([]byte(`{"formatVersion":"1","credentials":[],"value":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("err = %v", err)
	}
	_, err = ReadRegistry([]byte(`{"formatVersion":"1","credentials":[]} {}`))
	if err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Errorf("err = %v", err)
	}
}

func TestReadRegistryValidates(t *testing.T) {
	if _, err := ReadRegistry([]byte(`{"formatVersion":"1","credentials":[{"id":"a"}]}`)); err == nil {
		t.Error("an invalid registry was accepted")
	}
}

func TestEncodeDoesNotEscapeHTML(t *testing.T) {
	c := secret("a")
	c.Description = "a <b> & c"
	data, err := NewRegistry(c).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "a <b> & c") {
		t.Errorf("encoded: %s", data)
	}
}

func TestLogAppendKeepsOrder(t *testing.T) {
	r := NewRegistry(secret("a"), secret("b"))
	l := NewLog()
	add := func(id string, typ EventType, at string) {
		t.Helper()
		if err := l.Append(r, Event{Credential: id, Type: typ, At: date(at)}); err != nil {
			t.Fatal(err)
		}
	}
	add("b", EventCreated, "2026-02-01")
	add("a", EventCreated, "2026-01-01")
	add("a", EventVerified, "2026-02-01")
	add("a", EventRotated, "2026-03-01")
	add("a", EventVerified, "2026-02-01")
	var got []string
	for _, e := range l.Events {
		got = append(got, e.Credential+" "+string(e.Type)+" "+day(e.At))
	}
	want := []string{
		"a created 2026-01-01",
		"a verified 2026-02-01",
		"a verified 2026-02-01",
		"b created 2026-02-01",
		"a rotated 2026-03-01",
	}
	if !slices.Equal(got, want) {
		t.Errorf("order:\n got %v\nwant %v", got, want)
	}
	if err := l.Validate(r); err != nil {
		t.Error(err)
	}
	if h := l.History("a"); len(h) != 4 {
		t.Errorf("History(a) has %d events", len(h))
	}
}

func TestLogAppendRefusesBadEvents(t *testing.T) {
	r := NewRegistry(secret("a"))
	l := NewLog()
	cases := map[string]Event{
		"unknown credential":        {Credential: "z", Type: EventCreated, At: date("2026-01-01")},
		"needs expires":             {Credential: "a", Type: EventExpires, At: date("2026-01-01")},
		"has no expires":            {Credential: "a", Type: EventVerified, At: date("2026-01-01"), Expires: ptr(date("2027-01-01"))},
		"at is required":            {Credential: "a", Type: EventCreated},
		`type "renamed" is not one`: {Credential: "a", Type: "renamed", At: date("2026-01-01")},
	}
	for want, e := range cases {
		err := l.Append(r, e)
		if err == nil {
			t.Errorf("%s: accepted", want)
			continue
		}
		wantProblem(t, problemsOf(t, err), want)
	}
	if len(l.Events) != 0 {
		t.Errorf("log changed: %v", l.Events)
	}
}

func TestLogValidateFindsDisorder(t *testing.T) {
	r := NewRegistry(secret("a"))
	l := &Log{FormatVersion: FormatVersion, Events: []Event{
		{Credential: "a", Type: EventRotated, At: date("2026-02-01")},
		{Credential: "a", Type: EventCreated, At: date("2026-01-01")},
	}}
	wantProblem(t, problemsOf(t, l.Validate(r)), "not in order")
}

func TestRecord(t *testing.T) {
	r := NewRegistry(secret("a"))
	l := NewLog()
	for _, e := range []Event{
		{Credential: "a", Type: EventCreated, At: date("2026-01-01"), Expires: ptr(date("2026-06-01"))},
		{Credential: "a", Type: EventVerified, At: date("2026-02-01")},
		{Credential: "a", Type: EventExpires, At: date("2026-03-01"), Expires: ptr(date("2026-09-01"))},
	} {
		if err := l.Append(r, e); err != nil {
			t.Fatal(err)
		}
	}
	rec := l.Record("a")
	if rec.Created == nil || day(*rec.Created) != "2026-01-01" || rec.Rotated != nil {
		t.Errorf("created/rotated = %v/%v", rec.Created, rec.Rotated)
	}
	if day(*rec.Changed()) != "2026-01-01" {
		t.Errorf("changed = %v", rec.Changed())
	}
	if rec.Expires == nil || day(*rec.Expires) != "2026-09-01" {
		t.Errorf("expires = %v", rec.Expires)
	}
	if rec.Verified == nil || day(*rec.Verified) != "2026-02-01" {
		t.Errorf("verified = %v", rec.Verified)
	}
	// A rotation without an expiry clears it.
	if err := l.Append(r, Event{Credential: "a", Type: EventRotated, At: date("2026-04-01")}); err != nil {
		t.Fatal(err)
	}
	rec = l.Record("a")
	if rec.Expires != nil || day(*rec.Changed()) != "2026-04-01" {
		t.Errorf("after rotation: expires %v, changed %v", rec.Expires, rec.Changed())
	}
}

func TestLogRoundTripAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(secret("a"))
	missing, err := LoadLog(filepath.Join(dir, "none.json"), r)
	if err != nil || len(missing.Events) != 0 {
		t.Fatalf("missing log = %+v, %v", missing, err)
	}
	l := NewLog()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	if err := l.Append(r, Event{Credential: "a", Type: EventCreated, At: at, By: "ops", Note: "first"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events.json")
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLog(path, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || !got.Events[0].At.Equal(at) || got.Events[0].At.Location() != time.UTC {
		t.Errorf("events = %+v", got.Events)
	}
	if _, err := LoadLog(path, NewRegistry()); err == nil {
		t.Error("a log naming unknown credentials was accepted")
	}
}

func TestAssess(t *testing.T) {
	now := date("2026-06-01")
	scheduled := secret("a")
	onExpiry := secret("a")
	onExpiry.Rotation = Rotation{Policy: PolicyOnExpiry}
	onDemand := secret("a")
	onDemand.Rotation = Rotation{Policy: PolicyOnDemand}
	planned := secret("a")
	planned.Status = StatusPlanned
	retired := secret("a")
	retired.Status = StatusRetired
	federated := secret("a")
	federated.Generation = Generation{Method: MethodFederated}
	federated.Rotation = Rotation{Policy: PolicyOnDemand}

	created := func(s string) Record { return Record{Created: ptr(date(s))} }
	withExpiry := func(rec Record, s string) Record { rec.Expires = ptr(date(s)); return rec }

	cases := []struct {
		name string
		c    Credential
		rec  Record
		want State
		next string
	}{
		{"retired", retired, created("2026-01-01"), StateRetired, ""},
		{"revoked", scheduled, Record{Created: ptr(date("2026-01-01")), Revoked: ptr(date("2026-02-01"))}, StateRetired, ""},
		{"planned", planned, Record{}, StatePlanned, ""},
		{"unrecorded", scheduled, Record{Verified: ptr(date("2026-05-01"))}, StateUnrecorded, ""},
		{"overdue", scheduled, created("2026-03-03"), StateOverdue, "2026-06-01"},
		{"due", scheduled, created("2026-03-04"), StateDue, "2026-06-02"},
		{"ok scheduled", scheduled, created("2026-05-01"), StateOK, "2026-07-30"},
		{"rotation counts", scheduled, Record{Created: ptr(date("2025-01-01")), Rotated: ptr(date("2026-05-01"))}, StateOK, "2026-07-30"},
		{"expired", onExpiry, withExpiry(created("2026-01-01"), "2026-06-01"), StateExpired, "2026-06-01"},
		{"expiring", onExpiry, withExpiry(created("2026-01-01"), "2026-06-30"), StateExpiring, "2026-06-30"},
		{"ok expiry", onExpiry, withExpiry(created("2026-01-01"), "2026-07-02"), StateOK, "2026-07-02"},
		{"unknown expiry", onExpiry, created("2026-01-01"), StateUnknownExpiry, ""},
		{"expired beats overdue", scheduled, withExpiry(created("2025-01-01"), "2026-01-01"), StateExpired, "2025-04-01"},
		{"on demand", onDemand, created("2020-01-01"), StateOK, ""},
		{"federated", federated, Record{}, StateOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Assess(tc.c, tc.rec, now, DefaultWarn)
			if a.State != tc.want {
				t.Errorf("state = %s (%s), want %s", a.State, a.Reason, tc.want)
			}
			if a.Reason == "" {
				t.Error("no reason")
			}
			got := ""
			if a.Next != nil {
				got = day(*a.Next)
			}
			if got != tc.next {
				t.Errorf("next = %q, want %q", got, tc.next)
			}
		})
	}
}

func TestListsAreComplete(t *testing.T) {
	if len(States()) != 9 || len(EventTypes()) != 6 || len(Kinds()) != 13 || len(StoreTypes()) != 9 || len(Issuers()) != 8 {
		t.Error("a list changed size; update the documentation and the schema")
	}
	if len(NewRegistry(secret("a")).Credentials[0].Primary()) != 1 {
		t.Error("Primary")
	}
}

func TestExampleFilesAreValid(t *testing.T) {
	r, err := LoadRegistry(filepath.Join("examples", "basic", "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLog(filepath.Join("examples", "basic", "events.json"), r); err != nil {
		t.Fatal(err)
	}
}

func TestHandling(t *testing.T) {
	h := Handling{Generate: ActorHuman, Distribute: ActorSystem, Rotate: ActorHuman}
	if got := h.Manual(); !slices.Equal(got, []Stage{StageGenerate, StageRotate}) {
		t.Errorf("Manual = %v", got)
	}
	if h.Automatic() {
		t.Error("Automatic with two manual steps")
	}
	auto := Handling{Generate: ActorSystem, Distribute: ActorSystem, Rotate: ActorSystem}
	if !auto.Automatic() || len(auto.Manual()) != 0 {
		t.Error("an automatic credential")
	}
	if h.By("other") != "" {
		t.Error("By of an unknown stage")
	}
}

func TestReach(t *testing.T) {
	aws := secret("aws")
	aws.Reaches = []string{"pw", "key"}
	pw := secret("pw")
	dsn := secret("dsn")
	dsn.Generation = Generation{Method: MethodDerived}
	dsn.DependsOn = []string{"pw"}
	dsn.Rotation = Rotation{Policy: PolicyOnDemand}
	key := secret("key")
	key.Reaches = []string{"aws"} // a cycle
	root := secret("root")
	root.Reaches = []string{"aws"}
	root.Protections = []string{"MFA: hardware key"}
	root.Recovery = "The provider's account recovery."
	r := NewRegistry(aws, pw, dsn, key, root)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := r.Exposes("root"); !slices.Equal(got, []string{"aws", "key", "pw"}) {
		t.Errorf("Exposes(root) = %v", got)
	}
	if got := r.Exposes("dsn"); !slices.Equal(got, []string{"pw"}) {
		t.Errorf("Exposes(dsn) = %v", got)
	}
	if got := r.ExposedBy("pw"); !slices.Equal(got, []string{"aws", "dsn", "key", "root"}) {
		t.Errorf("ExposedBy(pw) = %v", got)
	}
	if got := r.Exposes("pw"); got != nil {
		t.Errorf("Exposes(pw) = %v", got)
	}
}

func TestValidateReachAndStores(t *testing.T) {
	c := secret("a")
	c.Reaches = []string{"a", "nope"}
	c.Protections = []string{" "}
	c.Stores[0].Console = "http://x"
	got := problemsOf(t, NewRegistry(c).Validate())
	for _, want := range []string{"reaches names itself", `reaches names unknown credential "nope"`, "protections[0] is empty", "stores[0].console must be an absolute https URL"} {
		wantProblem(t, got, want)
	}
}

func TestProgress(t *testing.T) {
	c := secret("a")
	c.Stores = append(c.Stores, Store{Type: StoreHostFile, Role: RoleRendered, Path: "/etc/a/env"})
	noRender := secret("b")
	fed := secret("c")
	fed.Generation = Generation{Method: MethodFederated}
	fed.Stores = []Store{{Type: StoreNone, Role: RoleRendered}}
	planned := secret("e")
	planned.Status = StatusPlanned
	retired := secret("d")
	retired.Status = StatusRetired
	at := func(s string) *time.Time { return ptr(date(s)) }
	cases := []struct {
		name string
		c    Credential
		rec  Record
		want Progress
	}{
		{"active, nothing recorded", c, Record{}, ProgressToRecord},
		{"planned, nothing recorded", planned, Record{}, ProgressToCreate},
		{"created, not deployed", c, Record{Created: at("2026-01-01")}, ProgressToDeploy},
		{"deployed", c, Record{Created: at("2026-01-01"), Deployed: at("2026-01-02")}, ProgressDeployed},
		{"rotated after deploy", c, Record{Created: at("2026-01-01"), Deployed: at("2026-01-02"), Rotated: at("2026-03-01")}, ProgressToDeploy},
		{"no rendered store", noRender, Record{Created: at("2026-01-01")}, ProgressDeployed},
		{"federated, not deployed", fed, Record{}, ProgressToCreate},
		{"federated, deployed", fed, Record{Deployed: at("2026-01-01")}, ProgressDeployed},
		{"retired", retired, Record{Created: at("2026-01-01")}, ProgressRetired},
		{"revoked", c, Record{Created: at("2026-01-01"), Revoked: at("2026-02-01")}, ProgressRetired},
	}
	for _, tc := range cases {
		if got := ProgressOf(tc.c, tc.rec); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
	if len(Progresses()) != 5 {
		t.Error("Progresses")
	}
}

// A planned credential created ahead of its launch is assessed like an
// active one.
func TestPlannedButCreated(t *testing.T) {
	c := secret("a")
	c.Status = StatusPlanned
	a := Assess(c, Record{Created: ptr(date("2026-05-01"))}, date("2026-06-01"), DefaultWarn)
	if a.State != StateOK {
		t.Errorf("state = %s", a.State)
	}
}

func TestDeployedEventHasNoExpiry(t *testing.T) {
	r := NewRegistry(secret("a"))
	err := NewLog().Append(r, Event{Credential: "a", Type: EventDeployed, At: date("2026-01-01"), Expires: ptr(date("2027-01-01"))})
	wantProblem(t, problemsOf(t, err), "a deployed event has no expires")
	l := NewLog()
	if err := l.Append(r, Event{Credential: "a", Type: EventDeployed, At: date("2026-01-01")}); err != nil {
		t.Fatal(err)
	}
	if rec := l.Record("a"); rec.Deployed == nil {
		t.Error("deployed not recorded")
	}
}
