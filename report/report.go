// Package report turns a registry and its event log into reports: a
// self-contained HTML page to search and filter, a Markdown reference
// and JSON.
package report

import (
	"cmp"
	"encoding/json"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/plexusone/systemspec-credential/credspec"
)

// Row is one credential with what the log says about it and its state.
type Row struct {
	Credential credspec.Credential `json:"credential"`
	Record     credspec.Record     `json:"record"`
	Assessment credspec.Assessment `json:"assessment"`
	History    []credspec.Event    `json:"history"`
	// Exposes are the credentials a holder of this one can reach, in all.
	Exposes []string `json:"exposes"`
	// ExposedBy are the credentials whose holders can reach this one.
	ExposedBy []string `json:"exposedBy"`
	// Progress is where the work stands: to create, to deploy, deployed.
	Progress credspec.Progress `json:"progress"`
	// NextStep says what to do next, when there is something to do.
	NextStep string `json:"nextStep,omitempty"`
}

// Report is every credential's row at a point in time.
type Report struct {
	Title    string                 `json:"title"`
	AsOf     time.Time              `json:"asOf"`
	WarnDays int                    `json:"warnDays"`
	Counts   map[credspec.State]int `json:"counts"`
	// Work counts the credentials at each progress.
	Work     map[credspec.Progress]int `json:"work"`
	Handling HandlingSummary           `json:"handling"`
	Rows     []Row                     `json:"rows"`
}

// HandlingSummary counts who does each lifecycle step, over the
// credentials that are not retired.
type HandlingSummary struct {
	// Total is the number of credentials counted.
	Total int `json:"total"`
	// Manual is how many have at least one step done by a person.
	Manual int `json:"manual"`
	// Automatic is how many have every step done by a system.
	Automatic int `json:"automatic"`
	// ByStage counts each stage's actors.
	ByStage map[credspec.Stage]map[credspec.Actor]int `json:"byStage"`
	// RotationsPerYear is how many times a year a person rotates
	// credentials on a schedule: the sum of 365 / interval.
	RotationsPerYear float64 `json:"rotationsPerYear"`
	// ExpiringByHand is how many credentials a person rotates when the
	// issuer's expiry comes.
	ExpiringByHand int `json:"expiringByHand"`
}

// Summarize counts who does each step of the credentials, leaving out
// retired ones.
func Summarize(creds []credspec.Credential) HandlingSummary {
	h := HandlingSummary{ByStage: map[credspec.Stage]map[credspec.Actor]int{}}
	for _, st := range credspec.Stages() {
		h.ByStage[st] = map[credspec.Actor]int{}
		for _, a := range credspec.Actors() {
			h.ByStage[st][a] = 0
		}
	}
	for _, c := range creds {
		if c.Status == credspec.StatusRetired {
			continue
		}
		h.Total++
		if c.Handling.Automatic() {
			h.Automatic++
		} else {
			h.Manual++
		}
		for _, st := range credspec.Stages() {
			h.ByStage[st][c.Handling.By(st)]++
		}
		if c.Handling.Rotate == credspec.ActorHuman {
			switch c.Rotation.Policy {
			case credspec.PolicyScheduled:
				h.RotationsPerYear += 365 / float64(c.Rotation.IntervalDays)
			case credspec.PolicyOnExpiry:
				h.ExpiringByHand++
			}
		}
	}
	h.RotationsPerYear = math.Round(h.RotationsPerYear*10) / 10
	return h
}

// Urgency orders states for attention: the ones needing action first,
// the ones needing none last.
func Urgency() []credspec.State {
	return []credspec.State{
		credspec.StateExpired, credspec.StateOverdue,
		credspec.StateExpiring, credspec.StateDue,
		credspec.StateUnrecorded, credspec.StateUnknownExpiry,
		credspec.StatePlanned, credspec.StateOK, credspec.StateRetired,
	}
}

// Build assesses every credential at now, warning within warn. A nil log
// is an empty one. Rows are ordered by urgency, then ID.
func Build(title string, r *credspec.Registry, l *credspec.Log, now time.Time, warn time.Duration) Report {
	if l == nil {
		l = credspec.NewLog()
	}
	rep := Report{
		Title:    title,
		AsOf:     now.UTC(),
		WarnDays: int(warn / (24 * time.Hour)),
		Counts:   map[credspec.State]int{},
		Work:     map[credspec.Progress]int{},
		Rows:     make([]Row, 0, len(r.Credentials)),
	}
	for _, c := range r.Credentials {
		rec := l.Record(c.ID)
		a := credspec.Assess(c, rec, now, warn)
		h := l.History(c.ID)
		if h == nil {
			h = []credspec.Event{}
		}
		p := credspec.ProgressOf(c, rec)
		rep.Rows = append(rep.Rows, Row{Credential: c, Record: rec, Assessment: a, History: h,
			Exposes: nonNil(r.Exposes(c.ID)), ExposedBy: nonNil(r.ExposedBy(c.ID)),
			Progress: p, NextStep: NextStep(c, p)})
		rep.Counts[a.State]++
		rep.Work[p]++
	}
	rep.Handling = Summarize(r.Credentials)
	order := Urgency()
	slices.SortStableFunc(rep.Rows, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(slices.Index(order, a.Assessment.State), slices.Index(order, b.Assessment.State)),
			cmp.Compare(a.Credential.ID, b.Credential.ID),
		)
	})
	return rep
}

// Filter returns the rows whose state is one of states.
func (rep Report) Filter(states ...credspec.State) []Row {
	var out []Row
	for _, row := range rep.Rows {
		if slices.Contains(states, row.Assessment.State) {
			out = append(out, row)
		}
	}
	return out
}

// JSON writes the report as indented JSON.
func (rep Report) JSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// NextStep says what to do next for a credential at a progress: where to
// create it and where to install it, or how it is deployed. It is empty
// when nothing is to do.
func NextStep(c credspec.Credential, p credspec.Progress) string {
	switch p {
	case credspec.ProgressToRecord:
		return "It exists but nothing is recorded: record it as created, with the date it was made (--at)."
	case credspec.ProgressToCreate:
		var parts []string
		where := strings.Join(slices.Compact(nonEmpty(c.Generation.Where, c.Issuer.Console)), ", ")
		create := "Create it"
		if where != "" {
			create += " in " + where
		}
		if c.Generation.Command != "" {
			create += " with: " + c.Generation.Command
		} else if len(c.Generation.Steps) > 0 {
			create += " following its steps"
		}
		if c.Generation.Method == credspec.MethodDerived {
			create = "Build it from " + strings.Join(c.DependsOn, ", ")
		}
		parts = append(parts, create+".")
		for _, s := range c.Stores {
			if s.Role != credspec.RoleRendered && s.Install != "" {
				parts = append(parts, "Install in "+strings.Join(nonEmpty(s.Type, s.Path), " ")+": "+sentence(s.Install))
			}
		}
		parts = append(parts, "Then record it as created.")
		return strings.Join(parts, " ")
	case credspec.ProgressToDeploy:
		var parts []string
		for _, s := range c.Stores {
			if s.Role == credspec.RoleRendered {
				how := s.Install
				if how == "" {
					how = "deploy it there"
				}
				parts = append(parts, "Deploy to "+strings.Join(nonEmpty(s.Type, s.Path), " ")+": "+sentence(how))
			}
		}
		parts = append(parts, "Then record it as deployed.")
		return strings.Join(parts, " ")
	}
	return ""
}

// sentence ends s with a full stop unless it ends with punctuation.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s[len(s)-1:], ".!?)") {
		return s
	}
	return s + "."
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
