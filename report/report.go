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
	"time"

	"github.com/plexusone/omnicredential"
)

// Row is one credential with what the log says about it and its state.
type Row struct {
	Credential omnicredential.Credential `json:"credential"`
	Record     omnicredential.Record     `json:"record"`
	Assessment omnicredential.Assessment `json:"assessment"`
	History    []omnicredential.Event    `json:"history"`
	// Exposes are the credentials a holder of this one can reach, in all.
	Exposes []string `json:"exposes"`
	// ExposedBy are the credentials whose holders can reach this one.
	ExposedBy []string `json:"exposedBy"`
}

// Report is every credential's row at a point in time.
type Report struct {
	Title    string                       `json:"title"`
	AsOf     time.Time                    `json:"asOf"`
	WarnDays int                          `json:"warnDays"`
	Counts   map[omnicredential.State]int `json:"counts"`
	Handling HandlingSummary              `json:"handling"`
	Rows     []Row                        `json:"rows"`
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
	ByStage map[omnicredential.Stage]map[omnicredential.Actor]int `json:"byStage"`
	// RotationsPerYear is how many times a year a person rotates
	// credentials on a schedule: the sum of 365 / interval.
	RotationsPerYear float64 `json:"rotationsPerYear"`
	// ExpiringByHand is how many credentials a person rotates when the
	// issuer's expiry comes.
	ExpiringByHand int `json:"expiringByHand"`
}

// Summarize counts who does each step of the credentials, leaving out
// retired ones.
func Summarize(creds []omnicredential.Credential) HandlingSummary {
	h := HandlingSummary{ByStage: map[omnicredential.Stage]map[omnicredential.Actor]int{}}
	for _, st := range omnicredential.Stages() {
		h.ByStage[st] = map[omnicredential.Actor]int{}
		for _, a := range omnicredential.Actors() {
			h.ByStage[st][a] = 0
		}
	}
	for _, c := range creds {
		if c.Status == omnicredential.StatusRetired {
			continue
		}
		h.Total++
		if c.Handling.Automatic() {
			h.Automatic++
		} else {
			h.Manual++
		}
		for _, st := range omnicredential.Stages() {
			h.ByStage[st][c.Handling.By(st)]++
		}
		if c.Handling.Rotate == omnicredential.ActorHuman {
			switch c.Rotation.Policy {
			case omnicredential.PolicyScheduled:
				h.RotationsPerYear += 365 / float64(c.Rotation.IntervalDays)
			case omnicredential.PolicyOnExpiry:
				h.ExpiringByHand++
			}
		}
	}
	h.RotationsPerYear = math.Round(h.RotationsPerYear*10) / 10
	return h
}

// Urgency orders states for attention: the ones needing action first,
// the ones needing none last.
func Urgency() []omnicredential.State {
	return []omnicredential.State{
		omnicredential.StateExpired, omnicredential.StateOverdue,
		omnicredential.StateExpiring, omnicredential.StateDue,
		omnicredential.StateUnrecorded, omnicredential.StateUnknownExpiry,
		omnicredential.StatePlanned, omnicredential.StateOK, omnicredential.StateRetired,
	}
}

// Build assesses every credential at now, warning within warn. A nil log
// is an empty one. Rows are ordered by urgency, then ID.
func Build(title string, r *omnicredential.Registry, l *omnicredential.Log, now time.Time, warn time.Duration) Report {
	if l == nil {
		l = omnicredential.NewLog()
	}
	rep := Report{
		Title:    title,
		AsOf:     now.UTC(),
		WarnDays: int(warn / (24 * time.Hour)),
		Counts:   map[omnicredential.State]int{},
		Rows:     make([]Row, 0, len(r.Credentials)),
	}
	for _, c := range r.Credentials {
		rec := l.Record(c.ID)
		a := omnicredential.Assess(c, rec, now, warn)
		h := l.History(c.ID)
		if h == nil {
			h = []omnicredential.Event{}
		}
		rep.Rows = append(rep.Rows, Row{Credential: c, Record: rec, Assessment: a, History: h,
			Exposes: nonNil(r.Exposes(c.ID)), ExposedBy: nonNil(r.ExposedBy(c.ID))})
		rep.Counts[a.State]++
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
func (rep Report) Filter(states ...omnicredential.State) []Row {
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

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
