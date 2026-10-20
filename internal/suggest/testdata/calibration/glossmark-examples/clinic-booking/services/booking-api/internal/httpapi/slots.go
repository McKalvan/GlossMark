package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *handlers) listSlots(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid practitioner id")
		return
	}
	from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		to = time.Now().Add(14 * 24 * time.Hour)
	}
	slots, err := h.svc.FindOpenSlots(r.Context(), id, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list slots")
		return
	}
	writeJSON(w, http.StatusOK, slots)
}

func (h *handlers) listPractitioners(w http.ResponseWriter, r *http.Request) {
	ps, err := h.practitioners.ListPractitioners(r.Context(), r.URL.Query().Get("specialty"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list practitioners")
		return
	}
	writeJSON(w, http.StatusOK, ps)
}
