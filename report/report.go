// Package report turns a registry and its event log into reports: a
// self-contained HTML page to search and filter, a Markdown reference
// and JSON.
package report

import (
	"cmp"
	"encoding/json"
	"io"
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
}

// Report is every credential's row at a point in time.
type Report struct {
	Title    string                       `json:"title"`
	AsOf     time.Time                    `json:"asOf"`
	WarnDays int                          `json:"warnDays"`
	Counts   map[omnicredential.State]int `json:"counts"`
	Rows     []Row                        `json:"rows"`
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
		rep.Rows = append(rep.Rows, Row{Credential: c, Record: rec, Assessment: a, History: h})
		rep.Counts[a.State]++
	}
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
