package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gomeznz/gpstrack/internal/gateway"
)

func do(h http.Handler, method, path, token, body string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestAccess(t *testing.T) {
	hub := gateway.NewHub(nil, nil)

	locked := (&Server{Hub: hub, Token: "secret"}).Handler()
	cases := []struct {
		method, path, token, body string
		want                      int
	}{
		{"GET", "/", "", "", 200},
		{"GET", "/static/leaflet/leaflet.js", "", "", 200},
		{"GET", "/api/info", "", "", 200},
		{"GET", "/api/devices", "", "", 401},
		{"GET", "/api/devices", "wrong", "", 401},
		{"GET", "/api/devices", "secret", "", 200},
		{"POST", "/api/devices/x/command", "", `{"preset":"cut"}`, 401},
		{"POST", "/api/devices/x/command", "secret", `{"preset":"cut"}`, 409}, // not connected
		{"POST", "/api/devices/x/command", "secret", `{"preset":"nope"}`, 400},
		{"POST", "/api/devices/x/command", "secret", `{}`, 400},
	}
	for _, c := range cases {
		if got := do(locked, c.method, c.path, c.token, c.body); got != c.want {
			t.Errorf("%s %s token=%q: %d, want %d", c.method, c.path, c.token, got, c.want)
		}
	}

	open := (&Server{Hub: hub}).Handler()
	if got := do(open, "GET", "/api/devices", "", ""); got != 200 {
		t.Errorf("open read: %d", got)
	}
	if got := do(open, "POST", "/api/devices/x/command", "", `{"preset":"cut"}`); got != 403 {
		t.Errorf("open command: %d, want 403", got)
	}
}
