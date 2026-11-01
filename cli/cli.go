// Package cli is omnicredential's command line as a Cobra command a host
// tool can mount over its own registry, such as one built in Go.
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	oscfs "github.com/grokify/oscompat/fs"
	"github.com/spf13/cobra"

	"github.com/plexusone/omnicredential"
	"github.com/plexusone/omnicredential/report"
	"github.com/plexusone/omnicredential/schema"
)

// Options configure the command.
type Options struct {
	// Use is the command's name, such as "credentials" when mounted.
	Use string
	// Short is the one-line help.
	Short string
	// Title is the reports' title; "Credentials" when empty.
	Title string
	// Registry builds the registry. When nil, the command reads the JSON
	// file named by its --registry flag.
	Registry func() (*omnicredential.Registry, error)
	// RegistryPath is --registry's default when Registry is nil.
	RegistryPath string
	// EventsPath is --events's default: the event log file.
	EventsPath string
	// Now returns the current time; time.Now when nil. Tests set it.
	Now func() time.Time
}

type env struct {
	opts     Options
	registry string
	events   string
}

// New returns the command tree.
func New(o Options) *cobra.Command {
	if o.Use == "" {
		o.Use = "omnicredential"
	}
	if o.Short == "" {
		o.Short = "Keep a registry of credentials without their values, record their lifecycle and report on them"
	}
	if o.Title == "" {
		o.Title = "Credentials"
	}
	if o.RegistryPath == "" {
		o.RegistryPath = "credentials.json"
	}
	if o.EventsPath == "" {
		o.EventsPath = "events.json"
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	e := &env{opts: o}
	cmd := &cobra.Command{Use: o.Use, Short: o.Short, SilenceUsage: true}
	if o.Registry == nil {
		cmd.PersistentFlags().StringVar(&e.registry, "registry", o.RegistryPath, "the registry file (JSON)")
	}
	cmd.PersistentFlags().StringVar(&e.events, "events", o.EventsPath, "the event log file (JSON); missing means no events yet")
	cmd.AddCommand(e.validate(), e.status(), e.report(), e.record(), e.export(), e.schemaCmd())
	return cmd
}

func (e *env) load() (*omnicredential.Registry, *omnicredential.Log, error) {
	var r *omnicredential.Registry
	var err error
	if e.opts.Registry != nil {
		if r, err = e.opts.Registry(); err != nil {
			return nil, nil, err
		}
		if err := r.Validate(); err != nil {
			return nil, nil, fmt.Errorf("registry: %w", err)
		}
	} else if r, err = omnicredential.LoadRegistry(e.registry); err != nil {
		return nil, nil, err
	}
	l, err := omnicredential.LoadLog(e.events, r)
	if err != nil {
		return nil, nil, err
	}
	return r, l, nil
}

func (e *env) validate() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check the registry and the event log",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			r, l, err := e.load()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%d credentials and %d events OK\n", len(r.Credentials), len(l.Events))
			return err
		},
	}
}

type when struct {
	asOf     string
	warnDays int
}

func (w *when) flags(c *cobra.Command) {
	c.Flags().StringVar(&w.asOf, "as-of", "", "assess at this date or RFC 3339 time instead of now")
	c.Flags().IntVar(&w.warnDays, "warn-days", int(omnicredential.DefaultWarn/(24*time.Hour)), "warn this many days before an expiry or a rotation date")
}

func (w *when) resolve(now func() time.Time) (time.Time, time.Duration, error) {
	if w.warnDays < 0 {
		return time.Time{}, 0, errors.New("--warn-days must not be negative")
	}
	warn := time.Duration(w.warnDays) * 24 * time.Hour
	if w.asOf == "" {
		return now(), warn, nil
	}
	t, err := parseTime(w.asOf)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("--as-of: %w", err)
	}
	return t, warn, nil
}

// parseTime reads a date (as midnight UTC) or an RFC 3339 time.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither a date (2006-01-02) nor an RFC 3339 time", s)
	}
	return t, nil
}

func parseStates(s string) ([]omnicredential.State, error) {
	var out []omnicredential.State
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		st := omnicredential.State(part)
		if !slices.Contains(omnicredential.States(), st) {
			return nil, fmt.Errorf("unknown state %q; states are %v", part, omnicredential.States())
		}
		out = append(out, st)
	}
	return out, nil
}

func (e *env) status() *cobra.Command {
	var w when
	var failOn, states, system, environment, progress string
	var all, manual bool
	c := &cobra.Command{
		Use:   "status",
		Short: "List credentials that need attention, and fail when any is in a --fail-on state",
		Long: "status lists every credential whose state is not ok, planned or retired, most urgent first.\n" +
			"It exits non-zero when any listed credential is in a --fail-on state, so deploy scripts and CI can use it as a gate.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			now, warn, err := w.resolve(e.opts.Now)
			if err != nil {
				return err
			}
			fail, err := parseStates(failOn)
			if err != nil {
				return fmt.Errorf("--fail-on: %w", err)
			}
			show, err := parseStates(states)
			if err != nil {
				return fmt.Errorf("--state: %w", err)
			}
			work := splitList(progress)
			for _, p := range work {
				if !slices.Contains(omnicredential.Progresses(), omnicredential.Progress(p)) {
					return fmt.Errorf("--progress: unknown %q; use %v", p, omnicredential.Progresses())
				}
			}
			r, l, err := e.load()
			if err != nil {
				return err
			}
			rep := report.Build(e.opts.Title, r, l, now, warn)
			quiet := []omnicredential.State{omnicredential.StateOK, omnicredential.StatePlanned, omnicredential.StateRetired}
			var rows []report.Row
			for _, row := range rep.Rows {
				st := row.Assessment.State
				switch {
				case system != "" && row.Credential.System != system,
					environment != "" && row.Credential.Environment != environment,
					len(show) > 0 && !slices.Contains(show, st),
					len(work) > 0 && !slices.Contains(work, string(row.Progress)),
					len(show) == 0 && len(work) == 0 && !all && slices.Contains(quiet, st),
					manual && row.Credential.Handling.Automatic():
					continue
				}
				rows = append(rows, row)
			}
			if err := statusTable(c.OutOrStdout(), rows, len(rep.Rows), now); err != nil {
				return err
			}
			failing := 0
			for _, row := range rows {
				if slices.Contains(fail, row.Assessment.State) {
					failing++
				}
			}
			if failing > 0 {
				return fmt.Errorf("%d credential(s) %s", failing, joinStates(fail))
			}
			return nil
		},
	}
	w.flags(c)
	c.Flags().StringVar(&failOn, "fail-on", "expired,overdue", "comma-separated states that make the command fail; empty never fails")
	c.Flags().StringVar(&states, "state", "", "comma-separated states to list (default: all but ok, planned and retired)")
	c.Flags().BoolVar(&all, "all", false, "list every credential")
	c.Flags().BoolVar(&manual, "manual", false, "only credentials with a step done by hand")
	c.Flags().StringVar(&progress, "progress", "", "comma-separated work to list: to-create, to-record, to-deploy, deployed, retired")
	c.Flags().StringVar(&system, "system", "", "only this system")
	c.Flags().StringVar(&environment, "env", "", "only this environment")
	return c
}

// matchIDs returns the ID itself, or with a *, every ID the pattern
// matches; it is an error when nothing matches.
func matchIDs(r *omnicredential.Registry, pattern string) ([]string, error) {
	if !strings.Contains(pattern, "*") {
		return []string{pattern}, nil
	}
	re, err := regexp.Compile("^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range r.Credentials {
		if re.MatchString(c.ID) {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no credential matches %q", pattern)
	}
	return ids, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinStates(states []omnicredential.State) string {
	parts := make([]string, len(states))
	for i, s := range states {
		parts[i] = string(s)
	}
	return strings.Join(parts, " or ")
}

func statusTable(w io.Writer, rows []report.Row, total int, now time.Time) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "WORK\tSTATE\tCREDENTIAL\tOWNER\tBY HAND\tREASON\n"); err != nil {
		return err
	}
	for _, row := range rows {
		byHand := "none"
		if m := row.Credential.Handling.Manual(); len(m) > 0 {
			parts := make([]string, len(m))
			for i, st := range m {
				parts[i] = string(st)
			}
			byHand = strings.Join(parts, ",")
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", row.Progress, row.Assessment.State, row.Credential.ID, row.Credential.Owner, byHand, row.Assessment.Reason); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%d of %d credentials listed, as of %s\n", len(rows), total, now.UTC().Format(time.DateOnly))
	return err
}

func (e *env) report() *cobra.Command {
	var w when
	var format, out, title, envOrder, sysOrder string
	var months, remind int
	c := &cobra.Command{
		Use:   "report",
		Short: "Write the HTML report, the Markdown reference, the management plan, a calendar or JSON",
		Long: "report writes one of five reports:\n\n" +
			"  html      one self-contained page to search and filter, with each credential's state and history\n" +
			"  markdown  a reference for documentation, from the registry only, so it does not change with time\n" +
			"  plan      the management plan: summary, launch checklist, rotation calendar, protections, exposure\n" +
			"  ics       the rotation calendar as an iCalendar file, with reminders, to import into a calendar\n" +
			"  json      the rows with their state, for other tools",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if title == "" {
				title = e.opts.Title
			}
			var buf bytes.Buffer
			switch format {
			case "markdown", "md":
				r, _, err := e.load()
				if err != nil {
					return err
				}
				if err := report.Markdown(&buf, title, r); err != nil {
					return err
				}
			case "html", "json", "plan", "ics":
				now, warn, err := w.resolve(e.opts.Now)
				if err != nil {
					return err
				}
				// Reports are assessed at the start of a day, so a page
				// written today is rebuilt byte for byte from its date.
				now = now.UTC().Truncate(24 * time.Hour)
				r, l, err := e.load()
				if err != nil {
					return err
				}
				rep := report.Build(title, r, l, now, warn)
				record := c.Parent().CommandPath() + " record"
				switch format {
				case "html":
					err = rep.HTML(&buf)
				case "plan":
					err = rep.Plan(&buf, r, report.PlanOptions{Months: months, EnvironmentOrder: splitList(envOrder), SystemOrder: splitList(sysOrder), RecordCommand: record})
				case "ics":
					err = rep.Calendar(&buf, report.CalendarOptions{Months: months, RemindDays: remind, RecordCommand: record})
				default:
					err = rep.JSON(&buf)
				}
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("--format %q: use html, markdown, plan, ics or json", format)
			}
			return write(c, out, buf.Bytes())
		},
	}
	w.flags(c)
	c.Flags().StringVarP(&format, "format", "f", "html", "html, markdown, plan, ics or json")
	c.Flags().IntVar(&months, "months", 12, "plan and ics: how many months ahead the calendar looks")
	c.Flags().StringVar(&envOrder, "environment-order", "", "plan: comma-separated environments in checklist order, such as prod,staging")
	c.Flags().StringVar(&sysOrder, "system-order", "", "plan: comma-separated systems in checklist order within an environment, such as shared infrastructure first")
	c.Flags().IntVar(&remind, "remind-days", 0, "ics: days before each date a reminder fires (default: --warn-days; negative for none)")
	c.Flags().StringVarP(&out, "output", "o", "", "file to write; standard output when empty")
	c.Flags().StringVar(&title, "title", "", "the report's title")
	return c
}

func (e *env) record() *cobra.Command {
	var at, expires, by, note string
	types := make([]string, len(omnicredential.EventTypes()))
	for i, t := range omnicredential.EventTypes() {
		types[i] = string(t)
	}
	c := &cobra.Command{
		Use:   "record <credential-id-or-pattern> <" + strings.Join(types, "|") + ">",
		Short: "Add an event to the log: a credential was created, rotated, deployed, verified, given an expiry or revoked",
		Long: "record adds an event for one credential, or for every credential whose ID matches a pattern,\n" +
			"where * matches any text, slashes included: record 'example-app/prod/*' deployed.",
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return types, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(c *cobra.Command, args []string) error {
			ev := omnicredential.Event{Type: omnicredential.EventType(args[1]), By: by, Note: note}
			ev.At = e.opts.Now()
			if at != "" {
				t, err := parseTime(at)
				if err != nil {
					return fmt.Errorf("--at: %w", err)
				}
				ev.At = t
			}
			if expires != "" {
				t, err := parseTime(expires)
				if err != nil {
					return fmt.Errorf("--expires: %w", err)
				}
				ev.Expires = &t
			}
			r, l, err := e.load()
			if err != nil {
				return err
			}
			ids, err := matchIDs(r, args[0])
			if err != nil {
				return err
			}
			for _, id := range ids {
				ev.Credential = id
				if err := l.Append(r, ev); err != nil {
					return err
				}
			}
			if err := l.Save(e.events); err != nil {
				return err
			}
			for _, id := range ids {
				if _, err := fmt.Fprintf(c.OutOrStdout(), "recorded %s %s at %s\n", id, ev.Type, ev.At.UTC().Format(time.RFC3339)); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%d event(s) in %s\n", len(ids), e.events)
			return err
		},
	}
	c.Flags().StringVar(&at, "at", "", "when it happened: a date or RFC 3339 time (default now)")
	c.Flags().StringVar(&expires, "expires", "", "the value's expiry: required for expires, optional for created and rotated")
	c.Flags().StringVar(&by, "by", "", "who did it")
	c.Flags().StringVar(&note, "note", "", "a note")
	return c
}

func (e *env) export() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "export",
		Short: "Write the registry as JSON",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			r, _, err := e.load()
			if err != nil {
				return err
			}
			data, err := r.Encode()
			if err != nil {
				return err
			}
			return write(c, out, data)
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "file to write; standard output when empty")
	return c
}

func (e *env) schemaCmd() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:       "schema <registry|events>",
		Short:     "Write the JSON Schema of the registry or the event log",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"registry", "events"},
		RunE: func(c *cobra.Command, args []string) error {
			data := schema.Registry()
			if args[0] == "events" {
				data = schema.Events()
			}
			return write(c, out, data)
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "file to write; standard output when empty")
	return c
}

func write(c *cobra.Command, path string, data []byte) error {
	if path == "" {
		_, err := c.OutOrStdout().Write(data)
		return err
	}
	// Reports and schemas hold no values and are meant to be read.
	return oscfs.WriteFile(path, data, oscfs.DefaultFilePerm)
}
