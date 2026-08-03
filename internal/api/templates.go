package api

import (
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
)

//go:embed templates/layout.html templates/login.html templates/dashboard.html templates/payments.html templates/sellers.html templates/settings.html templates/modals.html templates/icons.html
var templateFiles embed.FS

//go:embed templates/static
var staticFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.html"))

func render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("api: render template", "template", name, "error", err)
	}
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "templates/static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
