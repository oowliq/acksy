package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

// webhookView is a webhook as the API shows it: the URL may embed a
// credential (Discord, Slack), so only its origin is returned.
type webhookView struct {
	*domain.Webhook
	URL string `json:"url"`
}

func viewWebhook(w *domain.Webhook) webhookView { return webhookView{Webhook: w, URL: w.URLDisplay()} }

type webhookRequest struct {
	Name        *string               `json:"name"`
	URL         *string               `json:"url"`
	Format      *domain.WebhookFormat `json:"format"`
	Topics      *[]string             `json:"topics"`
	MinPriority *int                  `json:"min_priority"`
	Enabled     *bool                 `json:"enabled"`
}

func (in webhookRequest) input() service.WebhookInput {
	return service.WebhookInput{
		Name: in.Name, URL: in.URL, Format: in.Format, Topics: in.Topics, MinPriority: in.MinPriority, Enabled: in.Enabled,
	}
}

func (a *api) listWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := a.svc.ListWebhooks(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	items := make([]webhookView, len(hooks))
	for i := range hooks {
		items[i] = viewWebhook(&hooks[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *api) createWebhook(w http.ResponseWriter, r *http.Request) {
	var in webhookRequest
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	c, err := a.svc.CreateWebhook(r.Context(), principal(r), in.input())
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"webhook": viewWebhook(c.Webhook), "secret": c.Secret})
}

func (a *api) getWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := a.svc.GetWebhook(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, viewWebhook(hook))
}

func (a *api) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var in webhookRequest
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	hook, err := a.svc.UpdateWebhook(r.Context(), principal(r), chi.URLParam(r, "id"), in.input())
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, viewWebhook(hook))
}

func (a *api) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteWebhook(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) rotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	secret, err := a.svc.RotateWebhookSecret(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
}

func (a *api) testWebhook(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.TestWebhook(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *api) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	items, err := a.svc.ListWebhookDeliveries(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
