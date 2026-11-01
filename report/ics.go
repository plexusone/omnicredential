package report

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/plexusone/systemspec-credential/credspec"
)

// CalendarOptions shape the calendar.
type CalendarOptions struct {
	// Months is how far ahead it looks; 12 when zero.
	Months int
	// RemindDays is how many days before each date a reminder fires; the
	// report's warning window when zero, and no reminder when negative.
	RemindDays int
	// RecordCommand is the command that records an event, such as
	// "credspec record"; each event's description gives the full
	// command to run once it is done.
	RecordCommand string
}

// Calendar writes the schedule as an iCalendar (RFC 5545) file: one
// all-day event for each rotation and expiry, with a reminder, and the
// procedure, impact and console in its description. A late item is put on
// the report's date. Event IDs are stable, so importing a newer file
// updates the same events.
func (rep Report) Calendar(w io.Writer, o CalendarOptions) error {
	if o.Months <= 0 {
		o.Months = 12
	}
	if o.RemindDays == 0 {
		o.RemindDays = rep.WarnDays
	}
	if o.RecordCommand == "" {
		o.RecordCommand = "credspec record"
	}
	bw := bufio.NewWriter(w)
	c := &ics{w: bw}
	stamp := rep.AsOf.UTC().Format("20060102T150405Z")

	c.prop("BEGIN", "VCALENDAR")
	c.prop("VERSION", "2.0")
	c.prop("PRODID", "-//plexusone//systemspec-credential//EN")
	c.prop("CALSCALE", "GREGORIAN")
	c.prop("X-WR-CALNAME", text(rep.Title))
	c.prop("X-CREDSPEC-AS-OF", rep.AsOf.Format(time.DateOnly))
	for _, x := range rep.Schedule(rep.AsOf.AddDate(0, o.Months, 0)) {
		cr := x.Row.Credential
		day := x.Date
		if x.Late {
			day = rep.AsOf
		}
		sum := sha256.Sum256([]byte(cr.ID + "|" + string(x.Due) + "|" + x.Date.Format(time.DateOnly)))
		what := "Rotate " + cr.Name
		event := credspec.EventRotated
		if x.Due == DueExpires {
			what = cr.Name + " expires"
		}
		if x.Late {
			what = "Late: " + what + " (due " + x.Date.Format(time.DateOnly) + ")"
		}
		where := strings.Join(nonEmpty(cr.System, cr.Environment), " ")
		desc := []string{
			cr.ID,
			"By: " + string(x.By),
		}
		if p := cr.Rotation.Procedure; p != "" {
			desc = append(desc, "Procedure: "+p)
		}
		if p := cr.Rotation.Impact; p != "" {
			desc = append(desc, "Impact: "+p)
		}
		if cr.Issuer.Console != "" {
			desc = append(desc, "Console: "+cr.Issuer.Console)
		}
		desc = append(desc, fmt.Sprintf("Then: %s %s %s", o.RecordCommand, cr.ID, event))

		c.prop("BEGIN", "VEVENT")
		c.prop("UID", hex.EncodeToString(sum[:8])+"@systemspec-credential")
		c.prop("DTSTAMP", stamp)
		c.prop("DTSTART;VALUE=DATE", day.Format("20060102"))
		c.prop("DTEND;VALUE=DATE", day.AddDate(0, 0, 1).Format("20060102"))
		c.prop("SUMMARY", text(what+" ("+where+")"))
		c.prop("DESCRIPTION", text(strings.Join(desc, "\n")))
		c.prop("CATEGORIES", text(string(x.Due)))
		if o.RemindDays > 0 && !x.Late {
			c.prop("BEGIN", "VALARM")
			c.prop("ACTION", "DISPLAY")
			c.prop("TRIGGER", fmt.Sprintf("-P%dD", o.RemindDays))
			c.prop("DESCRIPTION", text(what))
			c.prop("END", "VALARM")
		}
		c.prop("END", "VEVENT")
	}
	c.prop("END", "VCALENDAR")
	if c.err != nil {
		return c.err
	}
	return bw.Flush()
}

// text escapes a TEXT value (RFC 5545, 3.3.11).
func text(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}

type ics struct {
	w   *bufio.Writer
	err error
}

// prop writes a content line, folded at 75 octets without splitting a
// UTF-8 character, ending in CRLF.
func (c *ics) prop(name, value string) {
	if c.err != nil {
		return
	}
	line := name + ":" + value
	var b strings.Builder
	n := 0
	for _, r := range line {
		size := len(string(r))
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	b.WriteString("\r\n")
	_, c.err = c.w.WriteString(b.String())
}
