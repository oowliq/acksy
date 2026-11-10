package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	apispec "github.com/oowliq/acksy/api"
)

// Every /api/v1 route must be documented in api/openapi.yaml and vice versa.
func TestOpenAPICoversRoutes(t *testing.T) {
	var spec struct {
		Servers []struct{ URL string } `yaml:"servers"`
		Paths   map[string]map[string]any
	}
	if err := yaml.Unmarshal(apispec.OpenAPI, &spec); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if len(spec.Servers) == 0 || spec.Servers[0].URL != "/api/v1" {
		t.Fatalf("unexpected servers: %+v", spec.Servers)
	}

	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			if method == "parameters" {
				continue
			}
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	h, _ := newAPIWith(t, Options{})
	routed := map[string]bool{}
	err := chi.Walk(h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/") {
			return nil
		}
		route = strings.TrimSuffix(strings.TrimPrefix(route, "/api/v1"), "/")
		routed[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var missing, stale []string
	for r := range routed {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for d := range documented {
		if !routed[d] {
			stale = append(stale, d)
		}
	}
	slices.Sort(missing)
	slices.Sort(stale)
	if len(missing) > 0 {
		t.Errorf("routes missing from openapi.yaml:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("documented but not routed:\n  %s", strings.Join(stale, "\n  "))
	}
}
