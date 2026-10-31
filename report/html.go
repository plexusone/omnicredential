package report

import (
	_ "embed"
	"html/template"
	"io"
	"time"

	"github.com/plexusone/omnicredential"
)

//go:embed report.html
var page string

var pageTemplate = template.Must(template.New("report").Parse(page))

// HTML writes the report as one self-contained page: it loads nothing
// from the network. The rows are embedded as JSON, which html/template
// escapes for the script element, and the page's script renders them
// with text nodes only.
func (rep Report) HTML(w io.Writer) error {
	return pageTemplate.Execute(w, struct {
		Title    string
		AsOf     string
		WarnDays int
		Total    int
		Rows     []Row
		Urgency  []omnicredential.State
	}{
		Title:    rep.Title,
		AsOf:     rep.AsOf.Format(time.DateOnly),
		WarnDays: rep.WarnDays,
		Total:    len(rep.Rows),
		Rows:     rep.Rows,
		Urgency:  Urgency(),
	})
}
