package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/domain"
)

// maxAdapterBody bounds adapter request bodies (the message limit plus
// form/JSON overhead); adapters enforce the exact limits.
const maxAdapterBody = domain.MaxBodyLen + 64*1024

// mountAdapters serves each ingest adapter under its prefix. Adapters accept
// API tokens only (no session cookies), so they need no CSRF protection, and
// they share token authentication and service-level authorization with the
// native API.
func (a *api) mountAdapters(r chi.Router, adapters []adapter.Adapter) {
	for _, ad := range adapters {
		h := a.adapterHandler(ad)
		r.Handle(ad.Prefix(), h)
		r.Handle(ad.Prefix()+"/*", h)
	}
}

func (a *api) adapterHandler(ad adapter.Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		r.Body = http.MaxBytesReader(w, r.Body, maxAdapterBody)
		fail := func(err error) {
			he := statusFor(err)
			if !he.expected {
				a.log.ErrorContext(ctx, "adapter request failed", "adapter", ad.Name(), "path", r.URL.Path, "err", err)
			}
			he.setHeaders(w)
			ad.WriteError(w, r, he.status, he.msg)
		}

		in, err := ad.Parse(r)
		if err != nil {
			fail(err)
			return
		}
		if in.Token == "" {
			fail(&domain.Error{Kind: domain.KindUnauthorized, Msg: "an Acksy API token is required"})
			return
		}
		p, err := a.authn.AuthenticateToken(ctx, in.Token)
		if err != nil {
			fail(err)
			return
		}

		res := adapter.Result{At: time.Now().UTC()}
		switch in.Op {
		case adapter.OpPublish:
			pr, err := a.svc.Publish(ctx, p, in.Message)
			if err != nil {
				fail(err)
				return
			}
			res.MessageID, res.Recipients = pr.MessageID, pr.Recipients
		case adapter.OpDelete, adapter.OpClear:
			action := domain.BulkDelete
			if in.Op == adapter.OpClear {
				action = domain.BulkRead
			}
			kr, err := a.svc.ApplyToKey(ctx, p, in.Message.Topic, in.Message.DedupKey, action)
			if err != nil {
				fail(err)
				return
			}
			res.Affected = kr.Affected
		}
		ad.WriteResult(w, r, in, res)
	}
}
