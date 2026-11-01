package omnicredential

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"time"
)

// EventType is what happened to a credential.
type EventType string

// Event types.
const (
	// EventCreated is the first value existing.
	EventCreated EventType = "created"
	// EventRotated is a new value replacing the old one.
	EventRotated EventType = "rotated"
	// EventExpires is the issuer setting or changing the expiry.
	EventExpires EventType = "expires"
	// EventDeployed is the value put in place where its consumers read
	// it: every store with role rendered, such as a server's files.
	EventDeployed EventType = "deployed"
	// EventVerified is someone confirming the value is present and works.
	EventVerified EventType = "verified"
	// EventRevoked is the value no longer working.
	EventRevoked EventType = "revoked"
)

// EventTypes lists every EventType.
func EventTypes() []EventType {
	return []EventType{EventCreated, EventRotated, EventExpires, EventDeployed, EventVerified, EventRevoked}
}

// Event is one thing that happened to a credential.
type Event struct {
	// Credential is the credential's ID.
	Credential string `json:"credential"`
	// Type is what happened.
	Type EventType `json:"type"`
	// At is when it happened.
	At time.Time `json:"at"`
	// Expires is the value's expiry: required for EventExpires, optional
	// for EventCreated and EventRotated, and not allowed otherwise.
	Expires *time.Time `json:"expires,omitempty"`
	// By is who did it.
	By string `json:"by,omitempty"`
	// Note adds detail.
	Note string `json:"note,omitempty"`
}

// Log is the append-only history of credentials.
type Log struct {
	// Schema is the JSON Schema the file names, if any.
	Schema string `json:"$schema,omitempty"`
	// FormatVersion is FormatVersion.
	FormatVersion string `json:"formatVersion"`
	// Events are sorted by Less.
	Events []Event `json:"events"`
}

// NewLog returns an empty log in the current format.
func NewLog() *Log { return &Log{FormatVersion: FormatVersion, Events: []Event{}} }

// compareEvents orders events by time, then credential, then type.
func compareEvents(a, b Event) int {
	return cmp.Or(
		a.At.Compare(b.At),
		cmp.Compare(a.Credential, b.Credential),
		cmp.Compare(slices.Index(EventTypes(), a.Type), slices.Index(EventTypes(), b.Type)),
	)
}

// Append validates the event against the registry and adds it in order.
// The log is unchanged when it returns an error.
func (l *Log) Append(r *Registry, e Event) error {
	var ps problems
	validateEvent(&ps, "event", e, r)
	if err := ps.err(); err != nil {
		return err
	}
	e.At = e.At.UTC()
	if e.Expires != nil {
		t := e.Expires.UTC()
		e.Expires = &t
	}
	i, _ := slices.BinarySearchFunc(l.Events, e, compareEvents)
	// A later event at the same instant goes after the existing ones.
	for i < len(l.Events) && compareEvents(l.Events[i], e) == 0 {
		i++
	}
	l.Events = slices.Insert(l.Events, i, e)
	return nil
}

// Validate checks the log's events against the registry and returns a
// *ValidationError listing every problem, or nil.
func (l *Log) Validate(r *Registry) error {
	var ps problems
	if l.FormatVersion != FormatVersion {
		ps.add("", "formatVersion is %q, want %q", l.FormatVersion, FormatVersion)
	}
	for i, e := range l.Events {
		validateEvent(&ps, fmt.Sprintf("events[%d]", i), e, r)
		if i > 0 && compareEvents(l.Events[i-1], e) > 0 {
			ps.add(fmt.Sprintf("events[%d]", i), "events are not in order")
		}
	}
	return ps.err()
}

func validateEvent(ps *problems, where string, e Event, r *Registry) {
	if e.Credential == "" {
		ps.add(where, "credential is required")
	} else if _, ok := r.Find(e.Credential); !ok {
		ps.add(where, "unknown credential %q", e.Credential)
	}
	closed(ps, where, "type", e.Type, EventTypes(), true)
	if e.At.IsZero() {
		ps.add(where, "at is required")
	}
	switch e.Type {
	case EventExpires:
		if e.Expires == nil {
			ps.add(where, "an expires event needs expires")
		}
	case EventDeployed, EventVerified, EventRevoked:
		if e.Expires != nil {
			ps.add(where, "a %s event has no expires", e.Type)
		}
	}
}

// Record is what the log says about one credential: the latest of each
// kind of event. Nil means never.
type Record struct {
	Created  *time.Time `json:"created,omitempty"`
	Rotated  *time.Time `json:"rotated,omitempty"`
	Expires  *time.Time `json:"expires,omitempty"`
	Deployed *time.Time `json:"deployed,omitempty"`
	Verified *time.Time `json:"verified,omitempty"`
	Revoked  *time.Time `json:"revoked,omitempty"`
}

// Changed is when the current value was made: the last rotation, or the
// creation.
func (r Record) Changed() *time.Time {
	if r.Rotated != nil {
		return r.Rotated
	}
	return r.Created
}

// Record returns what the log says about the credential.
func (l *Log) Record(id string) Record {
	var rec Record
	for _, e := range l.Events {
		if e.Credential != id {
			continue
		}
		at := e.At
		switch e.Type {
		case EventCreated:
			rec.Created = &at
			rec.Expires = e.Expires
		case EventRotated:
			rec.Rotated = &at
			rec.Expires = e.Expires
		case EventExpires:
			rec.Expires = e.Expires
		case EventDeployed:
			rec.Deployed = &at
		case EventVerified:
			rec.Verified = &at
		case EventRevoked:
			rec.Revoked = &at
		}
	}
	return rec
}

// History returns the credential's events, oldest first.
func (l *Log) History(id string) []Event {
	var out []Event
	for _, e := range l.Events {
		if e.Credential == id {
			out = append(out, e)
		}
	}
	return out
}

// ReadLog decodes a log, refusing unknown fields, and validates it
// against the registry.
func ReadLog(data []byte, r *Registry) (*Log, error) {
	var l Log
	if err := decodeStrict(data, &l); err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	if l.Events == nil {
		l.Events = []Event{}
	}
	if err := l.Validate(r); err != nil {
		return nil, err
	}
	return &l, nil
}

// LoadLog reads and validates the log at path. A missing file is an
// empty log, so a new registry needs no events file yet.
func LoadLog(path string, r *Registry) (*Log, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the caller names the events file.
	if os.IsNotExist(err) {
		return NewLog(), nil
	}
	if err != nil {
		return nil, err
	}
	l, err := ReadLog(data, r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// Encode returns the log as indented JSON.
func (l *Log) Encode() ([]byte, error) { return encode(l) }

// Save writes the log to path as indented JSON.
func (l *Log) Save(path string) error {
	data, err := l.Encode()
	if err != nil {
		return err
	}
	return writeFile(path, data)
}
