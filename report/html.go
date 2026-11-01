package report

import (
	_ "embed"
	"errors"
	"html/template"
	"io"
	"regexp"
	"time"

	"github.com/plexusone/systemspec-credential/credspec"
)

//go:embed report.html
var page string

var pageTemplate = template.Must(template.New("report").Parse(page))

// HTML writes the report as one self-contained page: it loads nothing
// from the network. The rows are embedded as JSON, which html/template
// escapes for the script element, and the page's script renders them
// with text nodes only.
func (rep Report) HTML(w io.Writer) error {
	type stage struct {
		Name  credspec.Stage
		Human int
	}
	var stages []stage
	for _, st := range credspec.Stages() {
		stages = append(stages, stage{st, rep.Handling.ByStage[st][credspec.ActorHuman]})
	}
	return pageTemplate.Execute(w, struct {
		Title    string
		AsOf     string
		WarnDays int
		Total    int
		Handling HandlingSummary
		Stages   []stage
		Rows     []Row
		Urgency  []credspec.State
	}{
		Title:    rep.Title,
		AsOf:     rep.AsOf.Format(time.DateOnly),
		WarnDays: rep.WarnDays,
		Total:    len(rep.Rows),
		Handling: rep.Handling,
		Stages:   stages,
		Rows:     rep.Rows,
		Urgency:  Urgency(),
	})
}

// asOfPattern finds the assessment date each time-dependent report
// records: the HTML page's meta element, the plan's comment and the
// calendar's property.
var asOfPattern = regexp.MustCompile(`(?:<meta name="credspec-as-of" content="|<!-- credspec-as-of: |X-CREDSPEC-AS-OF:)(\d{4}-\d{2}-\d{2})`)

// AsOf returns the date a report written by HTML, Plan or Calendar was
// assessed at. Building the report again at that date and comparing tells
// whether a committed copy is current with its registry and events.
func AsOf(page []byte) (time.Time, error) {
	m := asOfPattern.FindSubmatch(page)
	if m == nil {
		return time.Time{}, errors.New("report: no credspec as-of date in the file")
	}
	return time.Parse(time.DateOnly, string(m[1]))
}
