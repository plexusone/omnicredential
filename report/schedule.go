package report

import (
	"cmp"
	"slices"
	"time"

	"github.com/plexusone/systemspec-credential/credspec"
)

// Due is what comes due for a credential.
type Due string

// What comes due.
const (
	// DueRotate is a scheduled rotation.
	DueRotate Due = "rotate"
	// DueExpires is the issuer's expiry.
	DueExpires Due = "expires"
)

// Occurrence is one date a credential needs something.
type Occurrence struct {
	Date time.Time `json:"date"`
	Due  Due       `json:"due"`
	// Late is true when the date is already past at the report's time:
	// an overdue rotation or an expired value.
	Late bool `json:"late"`
	// By is who does the rotation.
	By  credspec.Actor `json:"by"`
	Row Row            `json:"-"`
}

// Schedule lists what comes due from the report's time until until: each
// scheduled rotation, repeating at its interval, and each expiry. A date
// already past is listed once, as late, on its own date. Credentials that
// are planned, retired, or have no recorded creation are left out: they
// have no dates yet. Occurrences are sorted by date, then ID.
func (rep Report) Schedule(until time.Time) []Occurrence {
	var out []Occurrence
	for _, row := range rep.Rows {
		c := row.Credential
		if c.Status != credspec.StatusActive || row.Record.Revoked != nil {
			continue
		}
		add := func(d time.Time, due Due) {
			out = append(out, Occurrence{Date: d, Due: due, Late: !d.After(rep.AsOf), By: c.Handling.Rotate, Row: row})
		}
		if changed := row.Record.Changed(); changed != nil && c.Rotation.Policy == credspec.PolicyScheduled {
			d := changed.AddDate(0, 0, c.Rotation.IntervalDays)
			if !d.After(rep.AsOf) {
				add(d, DueRotate)
			} else {
				for ; d.Before(until); d = d.AddDate(0, 0, c.Rotation.IntervalDays) {
					add(d, DueRotate)
				}
			}
		}
		if e := row.Record.Expires; e != nil && e.Before(until) {
			add(*e, DueExpires)
		}
	}
	slices.SortStableFunc(out, func(a, b Occurrence) int {
		return cmp.Or(a.Date.Compare(b.Date), cmp.Compare(a.Row.Credential.ID, b.Row.Credential.ID), cmp.Compare(a.Due, b.Due))
	})
	return out
}
