package api

import (
	"errors"
	"net/http"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type conceptRow struct {
	Concept        billing.Concept
	AmountDisplay  string
	AmountValue    string
	RecurringLabel string
}

type conceptOption struct {
	ID          int64
	Name        string
	AmountCents int64
	AmountValue string
	Currency    string
}

type conceptsView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Tenant   billing.Tenant
	Rows     []conceptRow
	Redirect string

	SessionExpired bool
}

func (s *Server) handleConcepts(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	concepts, err := s.repo.ListConcepts(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list concepts", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	view := conceptsView{
		T:        t,
		Title:    t.ConceptsPageTtl,
		Active:   "concepts",
		Error:    r.URL.Query().Get("error"),
		Tenant:   tenant,
		Redirect: "/concepts",

		SessionExpired: !tenant.HasValidSession(s.now()),
	}

	for _, concept := range concepts {
		recLabel := t.RecurringNo
		if concept.Recurring {
			recLabel = t.RecurringYes
		}
		row := conceptRow{
			Concept:        concept,
			AmountDisplay:  formatAmount(concept.AmountCents, concept.Currency),
			AmountValue:    centsValue(concept.AmountCents),
			RecurringLabel: recLabel,
		}
		view.Rows = append(view.Rows, row)
	}

	render(w, http.StatusOK, "concepts", view)
}

func (s *Server) parseConceptForm(r *http.Request) (billing.Concept, error) {
	var concept billing.Concept
	if err := r.ParseForm(); err != nil {
		return concept, errors.New("invalid form")
	}

	concept.Name = r.FormValue("name")
	if concept.Name == "" {
		return concept, errors.New("concept name is required")
	}
	concept.Slug = r.FormValue("slug")
	if v := r.FormValue("amount"); v != "" {
		cents, err := parseAmountCents(v)
		if err != nil {
			return concept, errors.New("invalid amount")
		}
		concept.AmountCents = cents
	}
	concept.Currency = r.FormValue("currency")
	if concept.Currency == "" {
		concept.Currency = "MXN"
	}
	concept.Recurring = r.FormValue("recurring") == "1"
	concept.Active = r.FormValue("active") != "0"
	concept.Note = r.FormValue("note")

	return concept, nil
}

func (s *Server) handleCreateConcept(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	concept, err := s.parseConceptForm(r)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	concept.TenantID = tenant.ID

	if _, err := s.repo.CreateConcept(r.Context(), concept); err != nil {
		s.logger.Error("api: create concept", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/concepts"), http.StatusSeeOther)
}

func (s *Server) handleUpdateConcept(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	conceptID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid concept id")
		return
	}

	concept, err := s.parseConceptForm(r)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	concept.ID = conceptID
	concept.TenantID = tenant.ID

	if _, err := s.repo.UpdateConcept(r.Context(), concept); errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "concept not found")
		return
	} else if err != nil {
		s.logger.Error("api: update concept", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/concepts"), http.StatusSeeOther)
}

func (s *Server) handleDeleteConcept(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	conceptID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid concept id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	err = s.repo.DeleteConcept(r.Context(), tenant.ID, conceptID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "concept not found")
		return
	}
	if err != nil {
		s.logger.Error("api: delete concept", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/concepts"), http.StatusSeeOther)
}

func (s *Server) conceptOptions(r *http.Request, tenantID int64) ([]conceptOption, map[int64]string, error) {
	concepts, err := s.repo.ListConcepts(r.Context(), tenantID)
	if err != nil {
		return nil, nil, err
	}

	options := make([]conceptOption, 0, len(concepts))
	names := make(map[int64]string, len(concepts))
	for _, concept := range concepts {
		names[concept.ID] = concept.Name
		if concept.Active {
			options = append(options, conceptOption{
				ID:          concept.ID,
				Name:        concept.Name,
				AmountCents: concept.AmountCents,
				AmountValue: centsValue(concept.AmountCents),
				Currency:    concept.Currency,
			})
		}
	}
	return options, names, nil
}
