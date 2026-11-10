package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

var errPushNotWired = domain.Conflict("Web Push is not available on this server")

func (a *api) pushConfig(w http.ResponseWriter, r *http.Request) {
	enabled := a.opts.PushPublicKey != "" && a.svc.PushEnabled(r.Context())
	key := ""
	if enabled {
		key = a.opts.PushPublicKey
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "public_key": key})
}

func (a *api) listPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := a.svc.ListPushSubscriptions(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": subs})
}

func (a *api) createPushSubscription(w http.ResponseWriter, r *http.Request) {
	if a.opts.PushPublicKey == "" {
		respondErr(w, r, a.log, errPushNotWired)
		return
	}
	// The shape of the browser's PushSubscription.toJSON(), plus settings.
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		ExpirationTime *int64 `json:"expirationTime"` // sent by browsers; unused
		Name           string `json:"name"`
		MinPriority    int    `json:"min_priority"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	sub, err := a.svc.SubscribePush(r.Context(), principal(r), service.SubscribePushInput{
		Endpoint: in.Endpoint, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth, Name: in.Name, MinPriority: in.MinPriority,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (a *api) updatePushSubscription(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        *string `json:"name"`
		MinPriority *int    `json:"min_priority"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	sub, err := a.svc.UpdatePushSubscription(r.Context(), principal(r), chi.URLParam(r, "id"),
		service.UpdatePushInput{Name: in.Name, MinPriority: in.MinPriority})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

func (a *api) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeletePushSubscription(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) testPushSubscription(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.TestPushSubscription(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
