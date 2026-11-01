package report

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/plexusone/omnicredential"
)

// PlanOptions shape the management plan.
type PlanOptions struct {
	// Months is how far ahead the rotation calendar looks; 12 when zero.
	Months int
	// EnvironmentOrder puts environments in this order in the launch
	// checklist, such as prod before staging; others follow by name. A
	// credential without an environment comes before them.
	EnvironmentOrder []string
	// SystemOrder puts systems in this order within an environment, such
	// as shared infrastructure first; others follow by name.
	SystemOrder []string
	// RecordCommand is the command that records an event, such as
	// "omnicredential record"; the plan appends the ID and the event.
	RecordCommand string
	// TopExposure is how many credentials the exposure table lists; 10
	// when zero.
	TopExposure int
}

// Plan writes the management plan as Markdown: a summary, the launch
// checklist of what people create by hand, the rotation calendar, the
// accounts' protections and the credentials whose leak exposes the most.
// It depends on the report's time, which it records in a comment so a
// committed copy can be rebuilt and compared.
func (rep Report) Plan(w io.Writer, r *omnicredential.Registry, o PlanOptions) error {
	if o.Months <= 0 {
		o.Months = 12
	}
	if o.TopExposure <= 0 {
		o.TopExposure = 10
	}
	if o.RecordCommand == "" {
		o.RecordCommand = "omnicredential record"
	}
	bw := bufio.NewWriter(w)
	m := &md{w: bw}
	asOf := rep.AsOf.Format(time.DateOnly)

	m.line("<!-- omnicredential-as-of: " + asOf + " -->")
	m.line("# " + rep.Title + ": Management Plan")
	m.blank()
	m.linef("As of %s. What exists, what you do by hand, and when each credential comes due. Generated from the registry and its event log; do not edit.", asOf)
	m.blank()

	planSummary(m, rep)
	planChecklist(m, rep, r, o)
	planRecord(m, rep, o)
	planDeploy(m, rep, o)
	planCalendar(m, rep, o)
	planAccounts(m, rep)
	planExposure(m, rep, o)

	if m.err != nil {
		return m.err
	}
	return bw.Flush()
}

func planSummary(m *md, rep Report) {
	status := map[omnicredential.Status]int{}
	for _, row := range rep.Rows {
		status[row.Credential.Status]++
	}
	h := rep.Handling
	m.line("## Summary")
	m.blank()
	m.line("| | |")
	m.line("|---|---|")
	m.linef("| Credentials | %d |", len(rep.Rows))
	m.linef("| Active / planned / retired | %d / %d / %d |", status[omnicredential.StatusActive], status[omnicredential.StatusPlanned], status[omnicredential.StatusRetired])
	m.linef("| Need a person for at least one step | %d |", h.Manual)
	m.linef("| Handled entirely by systems | %d |", h.Automatic)
	for _, st := range omnicredential.Stages() {
		m.linef("| %s by hand / by a system | %d / %d |", capital(string(st)), h.ByStage[st][omnicredential.ActorHuman], h.ByStage[st][omnicredential.ActorSystem])
	}
	m.linef("| Scheduled rotations by hand, per year | about %s |", strconv.FormatFloat(h.RotationsPerYear, 'f', -1, 64))
	m.linef("| Rotated by hand at the issuer's expiry | %s |", credentials(h.ExpiringByHand))
	m.linef("| To create / to record / to deploy / deployed | %d / %d / %d / %d |", rep.Work[omnicredential.ProgressToCreate], rep.Work[omnicredential.ProgressToRecord],
		rep.Work[omnicredential.ProgressToDeploy], rep.Work[omnicredential.ProgressDeployed])
	m.blank()

	type key struct{ system, env string }
	var keys []key
	counts := map[key]map[string]int{}
	for _, row := range rep.Rows {
		k := key{row.Credential.System, orDash(row.Credential.Environment)}
		if counts[k] == nil {
			counts[k] = map[string]int{}
			keys = append(keys, k)
		}
		counts[k]["all"]++
		counts[k][string(row.Credential.Status)]++
		if !row.Credential.Handling.Automatic() {
			counts[k]["hand"]++
		}
	}
	slices.SortFunc(keys, func(a, b key) int { return cmp.Or(strings.Compare(a.system, b.system), strings.Compare(a.env, b.env)) })
	m.line("### By System and Environment")
	m.blank()
	m.line("| System | Environment | Credentials | Active | Planned | By hand |")
	m.line("|---|---|---|---|---|---|")
	for _, k := range keys {
		c := counts[k]
		m.linef("| %s | %s | %d | %d | %d | %d |", cell(k.system), cell(k.env), c["all"], c["active"], c["planned"], c["hand"])
	}
	m.blank()

	m.line("### By State")
	m.blank()
	m.line("| State | Credentials |")
	m.line("|---|---|")
	for _, s := range Urgency() {
		if n := rep.Counts[s]; n > 0 {
			m.linef("| %s | %d |", s, n)
		}
	}
	m.blank()
}

// planChecklist lists, in order, what a person creates by hand: the
// credentials that are planned or active without a record.
func planChecklist(m *md, rep Report, r *omnicredential.Registry, o PlanOptions) {
	type group struct {
		title       string
		rank        int
		system, env string
		rows        []Row
	}
	envRank := func(env string) int {
		if env == "" {
			return -1
		}
		if i := slices.Index(o.EnvironmentOrder, env); i >= 0 {
			return i
		}
		return len(o.EnvironmentOrder)
	}
	var groups []*group
	find := func(title string, rank int, system, env string) *group {
		for _, g := range groups {
			if g.title == title {
				return g
			}
		}
		g := &group{title: title, rank: rank, system: system, env: env}
		groups = append(groups, g)
		return g
	}
	auto := 0
	// Registry order, so a credential's dependencies (declared before it
	// or not) are put first below.
	byID := map[string]Row{}
	for _, row := range rep.Rows {
		byID[row.Credential.ID] = row
	}
	for _, c := range r.Credentials {
		row := byID[c.ID]
		if row.Progress != omnicredential.ProgressToCreate {
			continue
		}
		if c.Handling.Generate != omnicredential.ActorHuman {
			auto++
			continue
		}
		g := find("Accounts", 0, "", "")
		if c.Kind != omnicredential.KindAccount {
			title := c.System
			if c.Environment != "" {
				title += " · " + c.Environment
			}
			g = find(title, 1, c.System, c.Environment)
		}
		g.rows = append(g.rows, row)
	}
	sysRank := func(system string) int {
		if i := slices.Index(o.SystemOrder, system); i >= 0 {
			return i
		}
		return len(o.SystemOrder)
	}
	slices.SortStableFunc(groups, func(a, b *group) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), cmp.Compare(envRank(a.env), envRank(b.env)),
			cmp.Compare(sysRank(a.system), sysRank(b.system)), strings.Compare(a.system, b.system), strings.Compare(a.env, b.env))
	})

	m.line("## Launch Checklist")
	m.blank()
	n := 0
	for _, g := range groups {
		n += len(g.rows)
	}
	intro := fmt.Sprintf("To create by hand: %s that do not exist yet.", credentials(n))
	if auto > 0 {
		intro += fmt.Sprintf(" Systems create %s more.", credentials(auto))
	}
	if n == 0 {
		m.line(intro)
		m.blank()
		return
	}
	m.line(intro + " Accounts come first, then the credentials of no one environment, then each environment in turn; within a group, a credential follows those it is built from. Record each one once it exists, so its rotation dates start.")
	m.blank()
	for i, g := range groups {
		m.linef("### %d. %s", i+1, g.title)
		m.blank()
		for _, row := range dependencyOrder(g.rows) {
			checklistItem(m, row, o)
		}
		m.blank()
	}
}

// dependencyOrder puts each row after the rows it depends on, keeping the
// given order otherwise.
func dependencyOrder(rows []Row) []Row {
	in := map[string]Row{}
	for _, r := range rows {
		in[r.Credential.ID] = r
	}
	done := map[string]bool{}
	var out []Row
	var visit func(r Row)
	visit = func(r Row) {
		if done[r.Credential.ID] {
			return
		}
		done[r.Credential.ID] = true
		for _, d := range r.Credential.DependsOn {
			if dep, ok := in[d]; ok {
				visit(dep)
			}
		}
		out = append(out, r)
	}
	for _, r := range rows {
		visit(r)
	}
	return out
}

func checklistItem(m *md, row Row, o PlanOptions) {
	c := row.Credential
	g := c.Generation
	m.linef("- [ ] **%s** (%s, %s)", c.ID, c.Kind, row.Assessment.State)
	sub := func(label, text string) {
		if strings.TrimSpace(text) != "" {
			m.linef("    - %s: %s", label, oneLine(text))
		}
	}
	sub("Create", strings.Join(nonEmpty(strings.Join(nonEmpty(c.Issuer.Type, c.Issuer.Account), ", "), g.Where, link("console", c.Issuer.Console)), "; "))
	if g.Method == omnicredential.MethodDerived {
		sub("Build it from", strings.Join(c.DependsOn, ", "))
	}
	if g.Command != "" {
		sub("Command", code(g.Command))
	}
	for i, s := range g.Steps {
		sub(fmt.Sprintf("Step %d", i+1), s)
	}
	for _, s := range c.Stores {
		dest := strings.Join(nonEmpty(s.Type, s.Account, code(s.Path)), " ")
		how := s.Install
		if how == "" {
			how = "(no install step given)"
		} else if looksLikeCommand(how) {
			how = code(how)
		}
		label := "Install"
		if s.Role == omnicredential.RoleRendered {
			label = "Then"
		} else if s.Role == omnicredential.RoleBackup {
			label = "Back up"
		}
		sub(label, dest+": "+how)
	}
	sub("Record", code(o.RecordCommand+" "+c.ID+" created"))
}

// planRecord lists the credentials that exist but have nothing recorded.
func planRecord(m *md, rep Report, o PlanOptions) {
	var rows []Row
	for _, row := range rep.Rows {
		if row.Progress == omnicredential.ProgressToRecord {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return strings.Compare(a.Credential.ID, b.Credential.ID) })
	m.line("## To Record")
	m.blank()
	m.linef("Already made, with nothing recorded: %s. Record when each was made, so its rotation dates start; a pattern records several at once.", credentials(len(rows)))
	m.blank()
	for _, row := range rows {
		m.linef("- [ ] %s", code(o.RecordCommand+" "+row.Credential.ID+" created --at <date>"))
	}
	m.blank()
}

// planDeploy lists the credentials created but not deployed since they
// last changed, with how each is deployed.
func planDeploy(m *md, rep Report, o PlanOptions) {
	var rows []Row
	for _, row := range rep.Rows {
		if row.Progress == omnicredential.ProgressToDeploy {
			rows = append(rows, row)
		}
	}
	m.line("## To Deploy")
	m.blank()
	if len(rows) == 0 {
		m.line("Nothing is waiting to be deployed.")
		m.blank()
		return
	}
	m.linef("Created or rotated, but not deployed since: %s. Deploy each, then record it.", credentials(len(rows)))
	m.blank()
	slices.SortStableFunc(rows, func(a, b Row) int { return strings.Compare(a.Credential.ID, b.Credential.ID) })
	for _, row := range rows {
		c := row.Credential
		m.linef("- [ ] **%s** (%s)", c.ID, strings.Join(nonEmpty(string(c.Handling.Distribute), c.Handling.Automation), ": "))
		for _, s := range c.Stores {
			if s.Role == omnicredential.RoleRendered {
				m.linef("    - To %s: %s", strings.Join(nonEmpty(s.Type, code(s.Path)), " "), oneLine(orDash(s.Install)))
			}
		}
		m.linef("    - Record: %s", code(o.RecordCommand+" "+c.ID+" deployed"))
	}
	m.blank()
}

func looksLikeCommand(s string) bool {
	first := strings.Fields(s)
	if len(first) == 0 || strings.Contains(s, ". ") {
		return false
	}
	w := first[0]
	return w == strings.ToLower(w) && !strings.HasSuffix(w, ",")
}

func planCalendar(m *md, rep Report, o PlanOptions) {
	until := rep.AsOf.AddDate(0, o.Months, 0)
	occ := rep.Schedule(until)
	undated := 0
	for _, row := range rep.Rows {
		c := row.Credential
		if c.Status == omnicredential.StatusRetired || row.Record.Revoked != nil {
			continue
		}
		if row.Record.Changed() == nil && (c.Rotation.Policy == omnicredential.PolicyScheduled || c.Rotation.Policy == omnicredential.PolicyOnExpiry) {
			undated++
		}
	}
	m.linef("## Rotation Calendar")
	m.blank()
	intro := fmt.Sprintf("What comes due in the next %d months: scheduled rotations and expiries, from what the event log records.", o.Months)
	if undated > 0 {
		intro += fmt.Sprintf(" %s with a schedule or an expiry have no dates yet: their creation is not recorded, and their dates start when it is.", capital(credentials(undated)))
	}
	m.line(intro)
	m.blank()
	if len(occ) == 0 {
		m.line("Nothing comes due in this period.")
		m.blank()
		return
	}
	month := ""
	for _, x := range occ {
		heading := x.Date.Format("2006-01")
		if x.Late {
			heading = "Late"
		}
		if heading != month {
			if month != "" {
				m.blank()
			}
			month = heading
			if x.Late {
				m.line("### Late: overdue or expired now")
			} else {
				m.line("### " + x.Date.Format("January 2006"))
			}
			m.blank()
			m.line("| Date | Credential | Due | By | Impact |")
			m.line("|---|---|---|---|---|")
		}
		impact := x.Row.Credential.Rotation.Impact
		m.linef("| %s | %s | %s | %s | %s |", x.Date.Format(time.DateOnly), cell(x.Row.Credential.ID), x.Due, x.By, cell(orDash(impact)))
	}
	m.blank()
}

func planAccounts(m *md, rep Report) {
	var rows []Row
	for _, row := range rep.Rows {
		c := row.Credential
		if c.Status != omnicredential.StatusRetired && (c.Kind == omnicredential.KindAccount || len(c.Protections) > 0) {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return strings.Compare(a.Credential.ID, b.Credential.ID) })
	m.line("## Account Protections")
	m.blank()
	m.line("What guards the accounts and the credentials that need more than secrecy. A box is ticked when a verified event is recorded for it.")
	m.blank()
	for _, row := range rows {
		c := row.Credential
		box := "[ ]"
		note := ""
		if v := row.Record.Verified; v != nil {
			box, note = "[x]", " (verified "+v.Format(time.DateOnly)+")"
		}
		m.linef("- %s **%s** (%s)%s", box, c.Name, c.ID, note)
		for _, p := range c.Protections {
			m.linef("    - %s", oneLine(p))
		}
		if c.Recovery != "" {
			m.linef("    - Recovery: %s", oneLine(c.Recovery))
		}
	}
	m.blank()
}

func planExposure(m *md, rep Report, o PlanOptions) {
	rows := slices.Clone(rep.Rows)
	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(cmp.Compare(len(b.Exposes), len(a.Exposes)), strings.Compare(a.Credential.ID, b.Credential.ID))
	})
	m.line("## Exposure")
	m.blank()
	m.line("The credentials whose leak exposes the most, through everything they reach. Protect these first.")
	m.blank()
	if len(rows) == 0 || len(rows[0].Exposes) == 0 {
		m.line("No credential reaches another.")
		m.blank()
		return
	}
	m.line("| Credential | Exposes | Exposed by | Protections |")
	m.line("|---|---|---|---|")
	for i, row := range rows {
		if i == o.TopExposure || len(row.Exposes) == 0 {
			break
		}
		m.linef("| %s | %d | %d | %s |", cell(row.Credential.ID), len(row.Exposes), len(row.ExposedBy), cell(orDash(strings.Join(row.Credential.Protections, "; "))))
	}
	m.blank()

	var lost []Row
	for _, row := range rep.Rows {
		c := row.Credential
		if c.Kind != omnicredential.KindAccount && c.Recovery != "" && c.Status != omnicredential.StatusRetired {
			lost = append(lost, row)
		}
	}
	if len(lost) == 0 {
		return
	}
	slices.SortStableFunc(lost, func(a, b Row) int { return strings.Compare(a.Credential.ID, b.Credential.ID) })
	m.line("### Recovery")
	m.blank()
	m.line("| Credential | Recovery | Backup |")
	m.line("|---|---|---|")
	for _, row := range lost {
		var backups []string
		for _, s := range row.Credential.Stores {
			if s.Role == omnicredential.RoleBackup {
				backups = append(backups, strings.Join(nonEmpty(s.Type, s.Path), " "))
			}
		}
		m.linef("| %s | %s | %s |", cell(row.Credential.ID), cell(row.Credential.Recovery), cell(orDash(strings.Join(backups, ", "))))
	}
	m.blank()
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
