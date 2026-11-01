package report

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/systemspec-credential/credspec"
)

func cred(id, system string, policy credspec.Policy, days int) credspec.Credential {
	return credspec.Credential{
		ID: id, Name: id[strings.LastIndex(id, "/")+1:], System: system, Environment: "prod",
		Kind: credspec.KindRandomSecret, Status: credspec.StatusActive, Owner: "ops",
		Issuer:     credspec.Issuer{Type: credspec.IssuerSelf},
		Generation: credspec.Generation{Method: credspec.MethodRandom, Command: "openssl rand -hex 32"},
		Stores:     []credspec.Store{{Type: credspec.StoreAWSSSM, Role: credspec.RolePrimary, Path: "/x/" + id}},
		Rotation:   credspec.Rotation{Policy: policy, IntervalDays: days},
		Handling: credspec.Handling{Generate: credspec.ActorHuman, Distribute: credspec.ActorSystem,
			Rotate: credspec.ActorHuman, Automation: "deploy"},
	}
}

func fixture(t *testing.T) (*credspec.Registry, *credspec.Log, time.Time) {
	t.Helper()
	ok := cred("b-app/prod/OK", "b-app", credspec.PolicyScheduled, 365)
	overdue := cred("a-app/prod/OLD", "a-app", credspec.PolicyScheduled, 30)
	overdue.Description = `</script><script>alert(1)</script> | pipe`
	overdue.Stores[0].Install = "aws ssm put-parameter --name /x/a-app/prod/OLD"
	overdue.Stores[0].Console = "https://console.example.com/ssm"
	overdue.Reaches = []string{"b-app/prod/OK"}
	overdue.Protections = []string{"MFA"}
	overdue.Recovery = "None: regenerate."
	planned := cred("a-app/staging/NEW", "a-app", credspec.PolicyOnDemand, 0)
	planned.Status = credspec.StatusPlanned
	planned.Handling = credspec.Handling{Generate: credspec.ActorSystem, Distribute: credspec.ActorSystem, Rotate: credspec.ActorSystem}
	r := credspec.NewRegistry(ok, overdue, planned)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	l := credspec.NewLog()
	for _, e := range []credspec.Event{
		{Credential: ok.ID, Type: credspec.EventCreated, At: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{Credential: overdue.ID, Type: credspec.EventCreated, At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), By: "ops"},
	} {
		if err := l.Append(r, e); err != nil {
			t.Fatal(err)
		}
	}
	return r, l, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
}

func TestBuildOrdersByUrgency(t *testing.T) {
	r, l, now := fixture(t)
	rep := Build("Credentials", r, l, now, credspec.DefaultWarn)
	var got []string
	for _, row := range rep.Rows {
		got = append(got, string(row.Assessment.State)+" "+row.Credential.ID)
	}
	want := "overdue a-app/prod/OLD,planned a-app/staging/NEW,ok b-app/prod/OK"
	if strings.Join(got, ",") != want {
		t.Errorf("rows = %v", got)
	}
	if !slices.Equal(rep.Rows[0].Exposes, []string{"b-app/prod/OK"}) || !slices.Equal(rep.Rows[2].ExposedBy, []string{"a-app/prod/OLD"}) || rep.Rows[1].Exposes == nil {
		t.Errorf("reach: %v %v", rep.Rows[0].Exposes, rep.Rows[2].ExposedBy)
	}
	// 365/30 + 365/365 by hand, scheduled.
	if h := rep.Handling; h.RotationsPerYear != 13.2 || h.ExpiringByHand != 0 {
		t.Errorf("rotations = %v, expiring = %d", h.RotationsPerYear, h.ExpiringByHand)
	}
	if h := rep.Handling; h.Total != 3 || h.Manual != 2 || h.Automatic != 1 || h.ByStage[credspec.StageRotate][credspec.ActorHuman] != 2 {
		t.Errorf("handling = %+v", rep.Handling)
	}
	if rep.Counts[credspec.StateOverdue] != 1 || rep.WarnDays != 30 {
		t.Errorf("counts = %v, warn = %d", rep.Counts, rep.WarnDays)
	}
	if len(rep.Filter(credspec.StateOverdue, credspec.StateExpired)) != 1 {
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
	if err := Build("Credentials", r, l, now, credspec.DefaultWarn).JSON(&b); err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(b.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Rows) != 3 || back.Rows[0].Assessment.State != credspec.StateOverdue {
		t.Errorf("round trip: %+v", back.Rows)
	}
}

func TestHTMLIsSelfContainedAndEscaped(t *testing.T) {
	r, l, now := fixture(t)
	var b bytes.Buffer
	if err := Build("Team <credentials>", r, l, now, credspec.DefaultWarn).HTML(&b); err != nil {
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
	if !strings.Contains(out, "By hand: <strong>2</strong> of 3 credentials need a person for at least one step (generate 2 · distribute 0 · rotate 2) · automatic: <strong>1</strong>") {
		t.Error("handling summary missing")
	}
	asOf, err := AsOf(b.Bytes())
	if err != nil || !asOf.Equal(now) {
		t.Errorf("AsOf = %v, %v", asOf, err)
	}
	if _, err := AsOf([]byte("<html>")); err == nil {
		t.Error("a page without a date")
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
		"## a-app\n",
		"| [OLD](#a-app-prod-old) | prod | random-secret | active | aws-ssm `/x/a-app/prod/OLD` | every 30 days | generate, rotate | ops |",
		`<a id="a-app-prod-old"></a>`,
		"### a-app/prod/OLD",
		"- **Command:** `openssl rand -hex 32`",
		"- **Stored (primary):** aws-ssm; `/x/a-app/prod/OLD`",
		"- **Rotation:** on-demand",
		"| [a-app](#a-app) | 2 | 1 | 1 | 0 | 1 | 1 |",
		"Of 3 credentials that are not retired, 2 need a person for at least one step and 1 are handled entirely by systems.",
		"| generate | 2 | 1 |",
		"| distribute | 0 | 3 |",
		"- **Handling:** generate by human, distribute by system, rotate by human (automation: deploy)",
		"Rotation by hand: about 13.2 scheduled rotations a year, and 0 credentials when their issuer's expiry comes.",
		"## By Hand, by Place\n",
		"### self\n",
		"- [a-app/prod/OLD](#a-app-prod-old) (active) → aws-ssm `/x/a-app/prod/OLD`",
		"- **Install (primary):** aws ssm put-parameter --name /x/a-app/prod/OLD",
		"[console](https://console.example.com/ssm)",
		"- **Reaches:** [b-app/prod/OK](#b-app-prod-ok)",
		"- **Exposes in all:** 1 credential",
		"- **Exposed by:** 1 credential",
		"- **Protections:** MFA",
		"- **Recovery:** None: regenerate.",
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
