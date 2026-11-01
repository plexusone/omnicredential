package report

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/omnicredential"
)

// planFixture adds a token with an expiry, an account and a credential
// built from another to the base fixture.
func planFixture(t *testing.T) (*omnicredential.Registry, Report) {
	t.Helper()
	r, l, now := fixture(t)
	tok := cred("a-app/ci/TOKEN", "a-app", omnicredential.PolicyOnExpiry, 0)
	tok.Environment = ""
	tok.Rotation.Impact = "CI stops, | until replaced"
	pw := cred("a-app/staging/PW", "a-app", omnicredential.PolicyOnDemand, 0)
	pw.Status = omnicredential.StatusPlanned
	pw.Environment = "staging"
	dsn := cred("a-app/staging/DSN", "a-app", omnicredential.PolicyOnDemand, 0)
	dsn.Status = omnicredential.StatusPlanned
	dsn.Environment = "staging"
	dsn.Generation = omnicredential.Generation{Method: omnicredential.MethodDerived}
	dsn.DependsOn = []string{pw.ID}
	acct := cred("org/owner", "org", omnicredential.PolicyOnDemand, 0)
	acct.Kind = omnicredential.KindAccount
	acct.Environment = ""
	acct.Status = omnicredential.StatusPlanned
	acct.Protections = []string{"MFA"}
	acct.Recovery = "Recovery codes."
	acct.Reaches = []string{tok.ID}
	keep := cred("a-app/prod/ROOT", "a-app", omnicredential.PolicyOnDemand, 0)
	keep.Recovery = "Only from the backup copy."
	keep.Stores = append(keep.Stores, omnicredential.Store{Type: omnicredential.StorePasswordManager, Role: omnicredential.RoleBackup, Path: "root copy"})
	// The dependent comes before its dependency in the registry.
	r.Add(tok, dsn, pw, acct, keep)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	exp := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	for _, e := range []omnicredential.Event{
		{Credential: tok.ID, Type: omnicredential.EventCreated, At: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), Expires: &exp},
		{Credential: keep.ID, Type: omnicredential.EventCreated, At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if err := l.Append(r, e); err != nil {
			t.Fatal(err)
		}
	}
	return r, Build("Team", r, l, now, omnicredential.DefaultWarn)
}

func TestSchedule(t *testing.T) {
	_, rep := planFixture(t)
	var got []string
	for _, x := range rep.Schedule(rep.AsOf.AddDate(1, 0, 0)) {
		got = append(got, x.Date.Format(time.DateOnly)+" "+x.Row.Credential.ID+" "+string(x.Due)+" "+map[bool]string{true: "late", false: "ahead"}[x.Late])
	}
	want := []string{
		"2026-01-31 a-app/prod/OLD rotate late",
		"2026-08-15 a-app/ci/TOKEN expires ahead",
		"2027-05-01 b-app/prod/OK rotate ahead",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("schedule:\n%s", strings.Join(got, "\n"))
	}
}

func TestPlan(t *testing.T) {
	r, rep := planFixture(t)
	var b bytes.Buffer
	if err := rep.Plan(&b, r, PlanOptions{EnvironmentOrder: []string{"prod", "staging"}, RecordCommand: "tool credentials record"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"<!-- omnicredential-as-of: 2026-06-01 -->\n# Team: Management Plan",
		"| Credentials | 8 |",
		"| Scheduled rotations by hand, per year | about 13.2 |",
		"| Rotated by hand at the issuer's expiry | 1 credential |",
		"| a-app | staging | 2 | 0 | 2 | 2 |",
		"To create by hand: 3 credentials with no created event: planned ones to make, and active ones that exist but are not recorded yet. Systems create 1 credential more.",
		"### 1. Accounts\n\n- [ ] **org/owner** (account, planned)",
		"### 2. a-app · staging",
		"    - Command: `openssl rand -hex 32`",
		"    - Install: aws-ssm `/x/a-app/staging/PW`: (no install step given)",
		"    - Record: `tool credentials record org/owner created`",
		"    - Build it from: a-app/staging/PW",
		"### Late: overdue or expired now",
		"| 2026-01-31 | a-app/prod/OLD | rotate | human | – |",
		"### August 2026",
		`| 2026-08-15 | a-app/ci/TOKEN | expires | human | CI stops, \| until replaced |`,
		"- [ ] **owner** (org/owner)\n    - MFA\n    - Recovery: Recovery codes.",
		"| org/owner | 1 | 0 | MFA |",
		"| To create / to deploy / deployed | 4 / 0 / 4 |",
		"## To Deploy\n\nNothing is waiting to be deployed.",
		"| a-app/prod/ROOT | Only from the backup copy. | password-manager root copy |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	// The password comes before the connection string built from it.
	if strings.Index(out, "**a-app/staging/PW**") > strings.Index(out, "**a-app/staging/DSN**") {
		t.Error("a credential is listed before what it is built from")
	}
	var ordered bytes.Buffer
	if err := rep.Plan(&ordered, r, PlanOptions{EnvironmentOrder: []string{"staging", "prod"}, SystemOrder: []string{"b-app", "a-app"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered.String(), "### 2. a-app · staging") {
		t.Errorf("environment order not followed:\n%s", ordered.String())
	}
	asOf, err := AsOf(b.Bytes())
	if err != nil || !asOf.Equal(rep.AsOf) {
		t.Errorf("AsOf = %v, %v", asOf, err)
	}
}

func TestCalendar(t *testing.T) {
	_, rep := planFixture(t)
	var b bytes.Buffer
	if err := rep.Calendar(&b, CalendarOptions{RecordCommand: "tool credentials record"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\n",
		"X-OMNICREDENTIAL-AS-OF:2026-06-01\r\n",
		// The late rotation goes on the report's date, without a reminder.
		"DTSTART;VALUE=DATE:20260601\r\n",
		"SUMMARY:Late: Rotate OLD (due 2026-01-31) (a-app prod)\r\n",
		"DTSTART;VALUE=DATE:20260815\r\nDTEND;VALUE=DATE:20260816\r\n",
		"SUMMARY:TOKEN expires (a-app)\r\n",
		"TRIGGER:-P30D\r\n",
		`Impact: CI stops\, | until replaced`,
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if n := strings.Count(out, "BEGIN:VEVENT"); n != 3 {
		t.Errorf("%d events", n)
	}
	if n := strings.Count(out, "BEGIN:VALARM"); n != 2 {
		t.Errorf("%d reminders", n)
	}
	for i, line := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("line %d is %d octets", i+1, len(line))
		}
		if strings.Contains(line, "\n") {
			t.Errorf("line %d has a bare newline", i+1)
		}
	}
	var none bytes.Buffer
	if err := rep.Calendar(&none, CalendarOptions{RemindDays: -1}); err != nil || strings.Contains(none.String(), "VALARM") {
		t.Errorf("reminders with RemindDays -1: %v", err)
	}
	if asOf, err := AsOf(b.Bytes()); err != nil || !asOf.Equal(rep.AsOf) {
		t.Errorf("AsOf = %v, %v", asOf, err)
	}
}

func TestFoldKeepsCharactersWhole(t *testing.T) {
	var b bytes.Buffer
	c := &ics{w: bufioWriter(&b)}
	c.prop("SUMMARY", strings.Repeat("é", 60))
	if err := c.w.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(b.String(), "\r\n"), "\r\n") {
		if len(line) > 75 || !strings.HasSuffix(line, "é") {
			t.Errorf("bad fold: %q", line)
		}
	}
}

func bufioWriter(b *bytes.Buffer) *bufio.Writer { return bufio.NewWriter(b) }

func TestProgressAndNextStep(t *testing.T) {
	r, l, now := fixture(t)
	c, _ := r.Find("b-app/prod/OK")
	c.Stores = append(c.Stores, omnicredential.Store{Type: omnicredential.StoreHostFile, Role: omnicredential.RoleRendered, Path: "/etc/b/env", Install: "the deploy tool writes it"})
	r.Credentials[0] = c
	rep := Build("Team", r, l, now, omnicredential.DefaultWarn)
	byID := map[string]Row{}
	for _, row := range rep.Rows {
		byID[row.Credential.ID] = row
	}
	if p := byID["b-app/prod/OK"]; p.Progress != omnicredential.ProgressToDeploy ||
		p.NextStep != "Deploy to host-file /etc/b/env: the deploy tool writes it. Then record it as deployed." {
		t.Errorf("OK: %s, %q", p.Progress, p.NextStep)
	}
	if p := byID["a-app/prod/OLD"]; p.Progress != omnicredential.ProgressDeployed || p.NextStep != "" {
		t.Errorf("OLD: %s, %q", p.Progress, p.NextStep)
	}
	if p := byID["a-app/staging/NEW"]; p.Progress != omnicredential.ProgressToCreate ||
		!strings.HasPrefix(p.NextStep, "Create it with: openssl rand -hex 32. ") || !strings.HasSuffix(p.NextStep, "Then record it as created.") {
		t.Errorf("NEW: %s, %q", p.Progress, p.NextStep)
	}
	if rep.Work[omnicredential.ProgressToDeploy] != 1 || rep.Work[omnicredential.ProgressToCreate] != 1 {
		t.Errorf("work = %v", rep.Work)
	}
	var b bytes.Buffer
	if err := rep.Plan(&b, r, PlanOptions{RecordCommand: "tool record"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## To Deploy\n\nCreated or rotated, but not deployed since: 1 credential.",
		"- [ ] **b-app/prod/OK** (system: deploy)\n    - To host-file `/etc/b/env`: the deploy tool writes it\n    - Record: `tool record b-app/prod/OK deployed`",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("plan missing %q", want)
		}
	}
	var h bytes.Buffer
	if err := rep.HTML(&h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.String(), `"progress":"to-deploy","nextStep":"Deploy to host-file /etc/b/env`) {
		t.Error("the page has no progress or next step")
	}
}

func TestNextStepForAnExistingUnrecordedCredential(t *testing.T) {
	c := cred("x/prod/K", "x", omnicredential.PolicyOnDemand, 0)
	if got := NextStep(c, omnicredential.ProgressToCreate); got != "It exists but nothing is recorded: record it as created, with its date." {
		t.Errorf("NextStep = %q", got)
	}
	if got := NextStep(c, omnicredential.ProgressDeployed); got != "" {
		t.Errorf("NextStep = %q", got)
	}
}

func TestNextStepNamesAPlaceOnce(t *testing.T) {
	c := cred("x/prod/C", "x", omnicredential.PolicyOnDemand, 0)
	c.Status = omnicredential.StatusPlanned
	c.Generation = omnicredential.Generation{Method: omnicredential.MethodIssued, Where: "https://console.example.com", Steps: []string{"a"}}
	c.Issuer.Console = "https://console.example.com"
	c.Stores[0].Install = "put it here"
	got := NextStep(c, omnicredential.ProgressToCreate)
	want := "Create it in https://console.example.com following its steps. Install in aws-ssm /x/x/prod/C: put it here. Then record it as created."
	if got != want {
		t.Errorf("NextStep = %q", got)
	}
}
