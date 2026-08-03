package api

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
)

//go:embed templates/layout.html templates/login.html templates/dashboard.html templates/payments.html templates/expenses.html templates/appointments.html templates/sellers.html templates/concepts.html templates/remissions.html templates/settings.html templates/modals.html templates/icons.html
var templateFiles embed.FS

//go:embed templates/static
var staticFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.html"))

// render builds the page in memory before sending any of it. Writing the
// status first and executing straight into the ResponseWriter meant a template
// that failed halfway still returned 200, so a missing field reached the
// operator as a blank page rather than as an error anyone would report.
func render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("api: render template", "template", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("api: write rendered page", "template", name, "error", err)
	}
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "templates/static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
