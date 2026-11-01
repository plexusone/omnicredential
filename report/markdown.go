package report

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/plexusone/omnicredential"
)

// Markdown writes a reference of the registry: a summary, then a section
// per system with a table and each credential's details. It uses only
// the registry, never the time or the events, so the same registry always
// gives the same page.
func Markdown(w io.Writer, title string, r *omnicredential.Registry) error {
	bw := bufio.NewWriter(w)
	m := &md{w: bw}

	m.line("# " + title)
	m.blank()
	m.line("Every credential, without values: what it is for, how and where it is made, what it may do, where it is stored, who uses it and how it rotates. Generated from the registry; do not edit.")
	m.blank()

	systems := groupBySystem(r)
	m.line("| System | Credentials | Planned | Active | Retired | By hand | Automatic |")
	m.line("|---|---|---|---|---|---|---|")
	for _, s := range systems {
		counts := map[omnicredential.Status]int{}
		for _, c := range s.creds {
			counts[c.Status]++
		}
		h := Summarize(s.creds)
		m.linef("| [%s](#%s) | %d | %d | %d | %d | %d | %d |", cell(s.name), anchor(s.name), len(s.creds),
			counts[omnicredential.StatusPlanned], counts[omnicredential.StatusActive], counts[omnicredential.StatusRetired],
			h.Manual, h.Automatic)
	}
	m.blank()

	h := Summarize(r.Credentials)
	m.line("## Who Does What")
	m.blank()
	m.linef("Of %d credentials that are not retired, %d need a person for at least one step and %d are handled entirely by systems.",
		h.Total, h.Manual, h.Automatic)
	m.blank()
	m.line("| Step | By hand | By a system |")
	m.line("|---|---|---|")
	for _, st := range omnicredential.Stages() {
		m.linef("| %s | %d | %d |", st, h.ByStage[st][omnicredential.ActorHuman], h.ByStage[st][omnicredential.ActorSystem])
	}
	m.blank()
	m.linef("Rotation by hand: about %s scheduled rotations a year, and %s when their issuer's expiry comes.",
		strconv.FormatFloat(h.RotationsPerYear, 'f', -1, 64), credentials(h.ExpiringByHand))
	m.blank()
	byPlace(m, r)

	for _, s := range systems {
		m.line("## " + s.name)
		m.blank()
		m.line("| Credential | Environment | Kind | Status | Primary store | Rotation | By hand | Owner |")
		m.line("|---|---|---|---|---|---|---|---|")
		for _, c := range s.creds {
			m.linef("| [%s](#%s) | %s | %s | %s | %s | %s | %s | %s |",
				cell(c.Name), anchor(c.ID), cell(orDash(c.Environment)), cell(c.Kind), c.Status,
				cell(storeSummary(c)), cell(rotationSummary(c.Rotation)), cell(manualSummary(c.Handling)), cell(c.Owner))
		}
		m.blank()
		for _, c := range s.creds {
			details(m, r, c)
		}
	}
	if err := m.err; err != nil {
		return err
	}
	return bw.Flush()
}

type system struct {
	name  string
	creds []omnicredential.Credential
}

func groupBySystem(r *omnicredential.Registry) []system {
	var out []system
	for _, c := range r.Credentials {
		i := slices.IndexFunc(out, func(s system) bool { return s.name == c.System })
		if i < 0 {
			out = append(out, system{name: c.System})
			i = len(out) - 1
		}
		out[i].creds = append(out[i].creds, c)
	}
	slices.SortStableFunc(out, func(a, b system) int { return strings.Compare(a.name, b.name) })
	return out
}

func details(m *md, r *omnicredential.Registry, c omnicredential.Credential) {
	m.line(`<a id="` + anchor(c.ID) + `"></a>`)
	m.blank()
	m.linef("### %s", c.ID)
	m.blank()
	if c.Description != "" {
		m.line(c.Description)
		m.blank()
	}
	field := func(name, value string) {
		if strings.TrimSpace(value) != "" {
			m.linef("- **%s:** %s", name, oneLine(value))
		}
	}
	field("Kind", c.Kind)
	field("Status", string(c.Status))
	field("Sensitivity", string(c.Sensitivity))
	field("Owner", c.Owner)
	field("Issuer", issuerSummary(c.Issuer))
	g := c.Generation
	field("Generated", strings.Join(nonEmpty(string(g.Method), g.Spec, g.Where), "; "))
	if g.Command != "" {
		field("Command", "`"+strings.ReplaceAll(g.Command, "`", "'")+"`")
	}
	for i, s := range g.Steps {
		field(fmt.Sprintf("Step %d", i+1), s)
	}
	for _, e := range c.Entitlements {
		field("Entitled", strings.Join(nonEmpty(e.Resource+": "+e.Access, e.Note), "; "))
	}
	for _, s := range c.Stores {
		field("Stored ("+string(s.Role)+")", strings.Join(nonEmpty(s.Type, s.Account, code(s.Path), s.Note, link("console", s.Console)), "; "))
		if s.Install != "" {
			field("Install ("+string(s.Role)+")", s.Install)
		}
	}
	for _, u := range c.Consumers {
		field("Used by", strings.Join(nonEmpty(strings.Join(nonEmpty(u.System, u.Component), " / "), u.Via, u.Note), "; "))
	}
	field("Handling", handlingSummary(c.Handling))
	field("Rotation", rotationSummary(c.Rotation))
	field("Protections", strings.Join(c.Protections, "; "))
	field("Recovery", c.Recovery)
	field("Rotation procedure", c.Rotation.Procedure)
	field("Rotation impact", c.Rotation.Impact)
	field("Revocation", c.Revocation)
	field("Exposure", c.Exposure)
	if len(c.DependsOn) > 0 {
		var deps []string
		for _, d := range c.DependsOn {
			deps = append(deps, fmt.Sprintf("[%s](#%s)", d, anchor(d)))
		}
		field("Depends on", strings.Join(deps, ", "))
	}
	if len(c.Reaches) > 0 {
		var reach []string
		for _, d := range c.Reaches {
			reach = append(reach, fmt.Sprintf("[%s](#%s)", d, anchor(d)))
		}
		field("Reaches", strings.Join(reach, ", "))
	}
	if n := len(r.Exposes(c.ID)); n > 0 {
		field("Exposes in all", credentials(n))
	}
	if n := len(r.ExposedBy(c.ID)); n > 0 {
		field("Exposed by", credentials(n))
	}
	field("Tags", strings.Join(c.Tags, ", "))
	for _, l := range c.Links {
		field("Link", fmt.Sprintf("[%s](%s)", strings.ReplaceAll(l.Title, "]", ")"), l.URL))
	}
	m.blank()
}

// byPlace lists the credentials a person generates, grouped by the issuer
// and account where they are made: a checklist for whoever sets them up.
func byPlace(m *md, r *omnicredential.Registry) {
	type place struct {
		name  string
		creds []omnicredential.Credential
	}
	var places []place
	for _, c := range r.Credentials {
		if c.Status == omnicredential.StatusRetired || c.Handling.Generate != omnicredential.ActorHuman {
			continue
		}
		name := strings.Join(nonEmpty(c.Issuer.Type, c.Issuer.Account), ": ")
		i := slices.IndexFunc(places, func(p place) bool { return p.name == name })
		if i < 0 {
			places = append(places, place{name: name})
			i = len(places) - 1
		}
		places[i].creds = append(places[i].creds, c)
	}
	if len(places) == 0 {
		return
	}
	slices.SortStableFunc(places, func(a, b place) int { return strings.Compare(a.name, b.name) })
	m.line("## By Hand, by Place")
	m.blank()
	m.line("Where each credential made by hand is created, grouped by issuer, with the store it goes to. Each one's details give the steps.")
	m.blank()
	for _, p := range places {
		m.line("### " + p.name)
		m.blank()
		for _, c := range p.creds {
			where := strings.Join(nonEmpty(link("console", c.Issuer.Console)), "")
			if where != "" {
				where = " · " + where
			}
			m.linef("- [%s](#%s) (%s) → %s%s", c.ID, anchor(c.ID), c.Status, storeSummary(c), where)
		}
		m.blank()
	}
}

func credentials(n int) string {
	if n == 1 {
		return "1 credential"
	}
	return fmt.Sprintf("%d credentials", n)
}

func link(title, url string) string {
	if url == "" {
		return ""
	}
	return fmt.Sprintf("[%s](%s)", title, url)
}

func storeSummary(c omnicredential.Credential) string {
	var parts []string
	for _, s := range c.Primary() {
		parts = append(parts, strings.Join(nonEmpty(s.Type, code(s.Path)), " "))
	}
	return orDash(strings.Join(parts, ", "))
}

// manualSummary lists the steps done by hand, or "none".
func manualSummary(h omnicredential.Handling) string {
	var parts []string
	for _, st := range h.Manual() {
		parts = append(parts, string(st))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func handlingSummary(h omnicredential.Handling) string {
	var parts []string
	for _, st := range omnicredential.Stages() {
		parts = append(parts, fmt.Sprintf("%s by %s", st, h.By(st)))
	}
	s := strings.Join(parts, ", ")
	if h.Automation != "" {
		s += " (automation: " + h.Automation + ")"
	}
	return s
}

func rotationSummary(r omnicredential.Rotation) string {
	if r.Policy == omnicredential.PolicyScheduled {
		return fmt.Sprintf("every %d days", r.IntervalDays)
	}
	return string(r.Policy)
}

func issuerSummary(i omnicredential.Issuer) string {
	s := strings.Join(nonEmpty(i.Type, i.Account), ", ")
	if i.Console != "" {
		s += fmt.Sprintf(" ([console](%s))", i.Console)
	}
	return s
}

func nonEmpty(in ...string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func code(s string) string {
	if s == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(s, "`", "'") + "`"
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cell makes text safe in a table cell.
func cell(s string) string { return strings.ReplaceAll(oneLine(s), "|", `\|`) }

// anchor turns an ID into an HTML id: lowercase letters, digits and
// hyphens.
func anchor(id string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(id) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

type md struct {
	w   *bufio.Writer
	err error
}

func (m *md) line(s string) {
	if m.err == nil {
		_, m.err = m.w.WriteString(s + "\n")
	}
}

func (m *md) linef(format string, args ...any) { m.line(fmt.Sprintf(format, args...)) }

func (m *md) blank() { m.line("") }
