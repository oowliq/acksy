package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// OIDC identities of a user. Issuer and subject travel in the body or query,
// not the path: subjects often contain "/" or "|".

func (a *api) listIdentities(w http.ResponseWriter, r *http.Request) {
	items, err := a.svc.ListIdentities(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *api) linkIdentity(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	ident, err := a.svc.LinkIdentity(r.Context(), principal(r), chi.URLParam(r, "id"), in.Issuer, in.Subject)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, ident)
}

func (a *api) unlinkIdentity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := a.svc.UnlinkIdentity(r.Context(), principal(r), chi.URLParam(r, "id"), q.Get("issuer"), q.Get("subject")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
