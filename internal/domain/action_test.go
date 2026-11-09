package domain

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPActionNormalize(t *testing.T) {
	m := Message{Body: "x", Actions: []Action{{Type: "http", Label: " Restart ", URL: "https://ha.lan/api", Method: "put"}}}
	if err := m.Normalize(time.Now()); err != nil {
		t.Fatal(err)
	}
	if a := m.Actions[0]; a.Label != "Restart" || a.Method != http.MethodPut {
		t.Fatalf("normalized: %+v", a)
	}
	m = Message{Body: "x", Actions: []Action{{Type: "http", Label: "Go", URL: "https://ha.lan/api"}}}
	_ = m.Normalize(time.Now())
	if m.Actions[0].Method != http.MethodPost {
		t.Fatalf("default method: %q", m.Actions[0].Method)
	}
}

func TestActionJSONHidesSecrets(t *testing.T) {
	in := `{"type":"http","label":"Open gate","url":"https://user:pw@ha.lan/api/services/cover/open?token=abc#x",
		"method":"POST","headers":{"Authorization":"Bearer SECRET"},"body":"{\"entity\":\"gate\"}","clear":true}`
	var a Action
	if err := json.Unmarshal([]byte(in), &a); err != nil {
		t.Fatal(err)
	}
	if a.Headers["Authorization"] != "Bearer SECRET" || a.Body == "" || !a.Clear || !strings.Contains(a.URL, "token=abc") {
		t.Fatalf("input lost data: %+v", a)
	}

	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET", "entity", "token=abc", "pw@", "user:"} {
		if strings.Contains(string(out), secret) {
			t.Fatalf("public JSON leaks %q: %s", secret, out)
		}
	}
	if !strings.Contains(string(out), `"url":"https://ha.lan/api/services/cover/open"`) || !strings.Contains(string(out), `"clear":true`) {
		t.Fatalf("public JSON: %s", out)
	}

	// Inside a notification, too (API responses, events, webhooks).
	nj, _ := json.Marshal(Notification{Actions: []Action{a}})
	if strings.Contains(string(nj), "SECRET") {
		t.Fatalf("notification JSON leaks: %s", nj)
	}

	// The store keeps everything and reads it back.
	stored, err := ActionsForStorage([]Action{a})
	if err != nil {
		t.Fatal(err)
	}
	var back []Action
	if err := json.Unmarshal(stored, &back); err != nil {
		t.Fatal(err)
	}
	if back[0].Headers["Authorization"] != "Bearer SECRET" || back[0].URL != a.URL || back[0].Body != a.Body {
		t.Fatalf("storage round trip: %+v", back[0])
	}

	// View actions keep their full URL: the client opens it.
	v, _ := json.Marshal(Action{Type: "view", Label: "Run", URL: "https://ci.example/run?id=42"})
	if !strings.Contains(string(v), "id=42") {
		t.Fatalf("view url trimmed: %s", v)
	}
}
