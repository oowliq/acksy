package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

const (
	defaultKeepAlive = 25 * time.Second
	wsWriteTimeout   = 10 * time.Second
)

// streamControl is the wire shape of ready/reset messages. Events are sent as
// domain.Event ({"seq","type","data","created_at"}); both share "type".
type streamControl struct {
	Type service.StreamKind `json:"type"`
	Seq  int64              `json:"seq"`
}

func encodeStream(m service.StreamMessage) ([]byte, error) {
	if m.Kind == service.StreamEvent {
		return json.Marshal(m.Event)
	}
	return json.Marshal(streamControl{Type: m.Kind, Seq: m.Seq})
}

// parseSince reads the resume position from ?since= or the Last-Event-ID
// header (sent automatically by EventSource on reconnect).
func parseSince(r *http.Request) (*int64, error) {
	v := r.URL.Query().Get("since")
	if v == "" {
		v = r.Header.Get("Last-Event-ID")
	}
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return nil, domain.Invalid("since must be a non-negative integer")
	}
	return &n, nil
}

func (a *api) keepAlive() time.Duration {
	if a.opts.KeepAlive > 0 {
		return a.opts.KeepAlive
	}
	return defaultKeepAlive
}

// events streams the caller's events as Server-Sent Events.
func (a *api) events(w http.ResponseWriter, r *http.Request) {
	since, err := parseSince(r)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	ctx := r.Context()
	ch, err := a.svc.Stream(ctx, principal(r), since)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}

	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil || rc.Flush() != nil {
		return
	}

	ping := time.NewTicker(a.keepAlive())
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case m, ok := <-ch:
			if !ok {
				return
			}
			data, err := encodeStream(m)
			if err != nil {
				a.log.Error("encode stream message", "err", err)
				return
			}
			// id lets EventSource resume via Last-Event-ID.
			if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", m.Seq, data); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

// websocketEvents streams the caller's events over a WebSocket. The stream is
// server-to-client only; mutations go through the REST API.
func (a *api) websocketEvents(w http.ResponseWriter, r *http.Request) {
	since, err := parseSince(r)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	ch, err := a.svc.Stream(ctx, principal(r), since)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}

	// Accept rejects cross-origin handshakes by default, which protects cookie
	// sessions from cross-site WebSocket hijacking.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept already wrote the HTTP error
	}
	defer func() { _ = c.CloseNow() }()
	ctx = c.CloseRead(ctx) // cancelled when the client goes away

	ping := time.NewTicker(a.keepAlive())
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		case m, ok := <-ch:
			if !ok {
				// Credential revoked or server shutting down: ask the client to
				// reconnect (it will be re-authenticated).
				_ = c.Close(websocket.StatusTryAgainLater, "stream ended, reconnect with since")
				return
			}
			data, err := encodeStream(m)
			if err != nil {
				a.log.Error("encode stream message", "err", err)
				return
			}
			wctx, wcancel := context.WithTimeout(ctx, wsWriteTimeout)
			err = c.Write(wctx, websocket.MessageText, data)
			wcancel()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log.Debug("websocket write", "err", err)
				}
				return
			}
		}
	}
}
