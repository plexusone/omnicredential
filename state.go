package omnicredential

import (
	"fmt"
	"time"
)

// State is where a credential stands at a point in time.
type State string

// States, most urgent first after the ones that need no action.
const (
	// StateRetired is retired or revoked.
	StateRetired State = "retired"
	// StatePlanned is needed but not created yet.
	StatePlanned State = "planned"
	// StateUnrecorded is active with no created or rotated event.
	StateUnrecorded State = "unrecorded"
	// StateExpired is past its expiry.
	StateExpired State = "expired"
	// StateOverdue is past its scheduled rotation.
	StateOverdue State = "overdue"
	// StateExpiring expires within the warning window.
	StateExpiring State = "expiring"
	// StateDue is due for rotation within the warning window.
	StateDue State = "due"
	// StateUnknownExpiry rotates on expiry but has no expiry recorded.
	StateUnknownExpiry State = "unknown-expiry"
	// StateOK needs nothing now.
	StateOK State = "ok"
)

// States lists every State in the order Assess checks them.
func States() []State {
	return []State{
		StateRetired, StatePlanned, StateUnrecorded, StateExpired, StateOverdue,
		StateExpiring, StateDue, StateUnknownExpiry, StateOK,
	}
}

// DefaultWarn is the default warning window before an expiry or a
// rotation date.
const DefaultWarn = 30 * 24 * time.Hour

// Assessment is a credential's state at a time.
type Assessment struct {
	State State `json:"state"`
	// Reason says why, in a sentence.
	Reason string `json:"reason"`
	// Next is the next date that matters: the expiry or the rotation
	// date, whichever is first.
	Next *time.Time `json:"next,omitempty"`
}

// Assess gives the credential's state at now from its record, warning
// within warn of an expiry or a rotation date.
func Assess(c Credential, rec Record, now time.Time, warn time.Duration) Assessment {
	switch {
	case c.Status == StatusRetired:
		return Assessment{State: StateRetired, Reason: "retired"}
	case rec.Revoked != nil:
		return Assessment{State: StateRetired, Reason: "revoked " + day(*rec.Revoked)}
	case c.Status == StatusPlanned:
		return Assessment{State: StatePlanned, Reason: "not created yet"}
	}
	changed := rec.Changed()
	if changed == nil && c.Generation.Method != MethodFederated {
		return Assessment{State: StateUnrecorded, Reason: "active, but no created or rotated event"}
	}

	var rotateBy *time.Time
	if c.Rotation.Policy == PolicyScheduled && changed != nil {
		t := changed.AddDate(0, 0, c.Rotation.IntervalDays)
		rotateBy = &t
	}
	next := earliest(rec.Expires, rotateBy)

	switch {
	case rec.Expires != nil && !now.Before(*rec.Expires):
		return Assessment{State: StateExpired, Reason: "expired " + day(*rec.Expires), Next: next}
	case rotateBy != nil && !now.Before(*rotateBy):
		return Assessment{State: StateOverdue, Reason: "rotation was due " + day(*rotateBy), Next: next}
	case rec.Expires != nil && now.Add(warn).After(*rec.Expires):
		return Assessment{State: StateExpiring, Reason: "expires " + day(*rec.Expires), Next: next}
	case rotateBy != nil && now.Add(warn).After(*rotateBy):
		return Assessment{State: StateDue, Reason: "rotation due " + day(*rotateBy), Next: next}
	case c.Rotation.Policy == PolicyOnExpiry && rec.Expires == nil:
		return Assessment{State: StateUnknownExpiry, Reason: "rotates on expiry, but no expiry is recorded"}
	}
	switch {
	case rotateBy != nil:
		return Assessment{State: StateOK, Reason: "next rotation " + day(*rotateBy), Next: next}
	case rec.Expires != nil:
		return Assessment{State: StateOK, Reason: "expires " + day(*rec.Expires), Next: next}
	case changed != nil:
		return Assessment{State: StateOK, Reason: fmt.Sprintf("%s since %s", c.Rotation.Policy, day(*changed))}
	}
	return Assessment{State: StateOK, Reason: "federated: no stored value"}
}

func earliest(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil || a.Before(*b):
		return a
	}
	return b
}

func day(t time.Time) string { return t.UTC().Format(time.DateOnly) }
