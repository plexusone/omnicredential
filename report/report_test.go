package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/omnicredential"
)

func cred(id, system string, policy omnicredential.Policy, days int) omnicredential.Credential {
	return omnicredential.Credential{
		ID: id, Name: id[strings.LastIndex(id, "/")+1:], System: system, Environment: "prod",
		Kind: omnicredential.KindRandomSecret, Status: omnicredential.StatusActive, Owner: "ops",
		Issuer:     omnicredential.Issuer{Type: omnicredential.IssuerSelf},
		Generation: omnicredential.Generation{Method: omnicredential.MethodRandom, Command: "openssl rand -hex 32"},
		Stores:     []omnicredential.Store{{Type: omnicredential.StoreAWSSSM, Role: omnicredential.RolePrimary, Path: "/x/" + id}},
		Rotation:   omnicredential.Rotation{Policy: policy, IntervalDays: days},
	}
}

func fixture(t *testing.T) (*omnicredential.Registry, *omnicredential.Log, time.Time) {
	t.Helper()
	ok := cred("b-app/prod/OK", "b-app", omnicredential.PolicyScheduled, 365)
	overdue := cred("a-app/prod/OLD", "a-app", omnicredential.PolicyScheduled, 30)
	overdue.Description = `</script><script>alert(1)</script> | pipe`
	planned := cred("a-app/staging/NEW", "a-app", omnicredential.PolicyOnDemand, 0)
	planned.Status = omnicredential.StatusPlanned
	r := omnicredential.NewRegistry(ok, overdue, planned)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	l := omnicredential.NewLog()
	for _, e := range []omnicredential.Event{
		{Credential: ok.ID, Type: omnicredential.EventCreated, At: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{Credential: overdue.ID, Type: omnicredential.EventCreated, At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), By: "ops"},
	} {
		if err := l.Append(r, e); err != nil {
			t.Fatal(err)
		}
	}
	return r, l, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
}

func TestBuildOrdersByUrgency(t *testing.T) {
	r, l, now := fixture(t)
	rep := Build("Credentials", r, l, now, omnicredential.DefaultWarn)
	var got []string
	for _, row := range rep.Rows {
		got = append(got, string(row.Assessment.State)+" "+row.Credential.ID)
	}
	want := "overdue a-app/prod/OLD,planned a-app/staging/NEW,ok b-app/prod/OK"
	if strings.Join(got, ",") != want {
		t.Errorf("rows = %v", got)
	}
	if rep.Counts[omnicredential.StateOverdue] != 1 || rep.WarnDays != 30 {
		t.Errorf("counts = %v, warn = %d", rep.Counts, rep.WarnDays)
	}
	if len(rep.Filter(omnicredential.StateOverdue, omnicredential.StateExpired)) != 1 {
		t.Error("Filter")
	}
	if len(rep.Rows[0].History) != 1 || rep.Rows[1].History == nil {
		t.Error("history")
	}
	if len(Build("x", r, nil, now, 0).Rows) != 3 {
		t.Error("a nil log")
	}
}

func TestJSON(t *testing.T) {
	r, l, now := fixture(t)
	var b bytes.Buffer
	if err := Build("Credentials", r, l, now, omnicredential.DefaultWarn).JSON(&b); err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(b.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Rows) != 3 || back.Rows[0].Assessment.State != omnicredential.StateOverdue {
		t.Errorf("round trip: %+v", back.Rows)
	}
}

func TestHTMLIsSelfContainedAndEscaped(t *testing.T) {
	r, l, now := fixture(t)
	var b bytes.Buffer
	if err := Build("Team <credentials>", r, l, now, omnicredential.DefaultWarn).HTML(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "<script>alert(1)") {
		t.Error("a description closed the data element")
	}
	if !strings.Contains(out, `</script>`) {
		t.Error("the description was not escaped as JSON")
	}
	if !strings.Contains(out, "<title>Team &lt;credentials&gt;</title>") {
		t.Error("the title was not escaped")
	}
	for _, ref := range []string{"src=", "href=\"http", "@import", "url("} {
		if strings.Contains(out, ref) {
			t.Errorf("the page refers to something external: %q", ref)
		}
	}
	if strings.Count(out, "<script") != 2 {
		t.Errorf("%d script elements", strings.Count(out, "<script"))
	}
	start := strings.Index(out, `id="data">`) + len(`id="data">`)
	end := strings.Index(out[start:], "</script>")
	var rows []Row
	if err := json.Unmarshal([]byte(out[start:start+end]), &rows); err != nil {
		t.Fatalf("embedded data: %v", err)
	}
	if len(rows) != 3 || !strings.HasPrefix(rows[0].Credential.Description, "</script>") {
		t.Errorf("embedded rows: %+v", rows)
	}
	if !strings.Contains(out, `const urgency = ["expired","overdue"`) {
		t.Error("urgency order not embedded")
	}
}

func TestMarkdown(t *testing.T) {
	r, _, _ := fixture(t)
	var b bytes.Buffer
	if err := Markdown(&b, "Credentials", r); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"# Credentials\n",
		"| [a-app](#a-app) | 2 | 1 | 1 | 0 |",
		"## a-app\n",
		"| [OLD](#a-app-prod-old) | prod | random-secret | active | aws-ssm `/x/a-app/prod/OLD` | every 30 days | ops |",
		`<a id="a-app-prod-old"></a>`,
		"### a-app/prod/OLD",
		"- **Command:** `openssl rand -hex 32`",
		"- **Stored (primary):** aws-ssm; `/x/a-app/prod/OLD`",
		"- **Rotation:** on-demand",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(out, "## a-app") > strings.Index(out, "## b-app") {
		t.Error("systems are not sorted")
	}
	var again bytes.Buffer
	if err := Markdown(&again, "Credentials", r); err != nil {
		t.Fatal(err)
	}
	if again.String() != out {
		t.Error("the page is not deterministic")
	}
}

func TestAnchorAndCell(t *testing.T) {
	cases := map[string]string{"A-App/prod/SESSION_KEY": "a-app-prod-session-key", "--x--": "x", "a//b": "a-b"}
	for in, want := range cases {
		if got := anchor(in); got != want {
			t.Errorf("anchor(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cell("a | b\nc"); got != `a \| b c` {
		t.Errorf("cell = %q", got)
	}
}
