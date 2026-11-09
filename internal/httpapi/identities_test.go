package httpapi

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/oowliq/acksy/internal/domain"
)

func TestAPIIdentities(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	bob := login(t, h, "bob", "password123")

	users := decode[struct{ Items []map[string]any }](t, root.do(http.MethodGet, "/api/v1/users", nil)).Items
	var bobID, rootID string
	for _, u := range users {
		switch u["username"] {
		case "bob":
			bobID = u["id"].(string)
		case "root":
			rootID = u["id"].(string)
			if u["has_password"] != true {
				t.Fatalf("root has_password: %v", u)
			}
		}
	}

	path := "/api/v1/users/" + bobID + "/identities"
	ident := map[string]string{"issuer": "https://idp.example", "subject": "auth0|a/b"}
	expect(t, root.do(http.MethodPost, path, ident), http.StatusCreated)
	list := decode[struct{ Items []domain.Identity }](t, root.do(http.MethodGet, path, nil)).Items
	if len(list) != 1 || list[0].Subject != "auth0|a/b" {
		t.Fatalf("identities: %+v", list)
	}
	expect(t, root.do(http.MethodPost, path, ident), http.StatusConflict)

	q := url.Values{"issuer": {"https://idp.example"}, "subject": {"auth0|a/b"}}
	expect(t, root.do(http.MethodDelete, path+"?"+q.Encode(), nil), http.StatusNoContent)
	expect(t, root.do(http.MethodDelete, path+"?"+q.Encode(), nil), http.StatusNotFound)

	expect(t, bob.do(http.MethodGet, "/api/v1/users/"+rootID+"/identities", nil), http.StatusForbidden)
	expect(t, root.do(http.MethodGet, "/api/v1/users/nope/identities", nil), http.StatusNotFound)
}
