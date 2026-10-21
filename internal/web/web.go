// Package web serves the Phase 0 status page and its small JSON API.
package web

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gomeznz/gpstrack/internal/gateway"
)

//go:embed index.html static
var static embed.FS

type Server struct {
	Hub *gateway.Hub
	// Token protects the API. Empty = read-only, commands disabled.
	Token string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.FileServerFS(static))
	mux.Handle("GET /static/", http.FileServerFS(static))
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/devices", s.auth(s.devices))
	mux.HandleFunc("GET /api/log", s.auth(s.log))
	mux.HandleFunc("POST /api/devices/{id}/command", s.auth(s.command))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
				httpError(w, http.StatusUnauthorized, "token required")
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"tokenRequired":   s.Token != "",
		"commandsEnabled": s.Token != "",
		"presets":         gateway.PresetList(),
		"serverTime":      time.Now().UTC(),
	})
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Hub.Devices())
}

func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 200
	}
	writeJSON(w, s.Hub.Recent(n))
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" {
		httpError(w, http.StatusForbidden, "commands are disabled: set GPSTRACK_TOKEN and restart")
		return
	}
	var req struct {
		Preset string `json:"preset"`
		Raw    string `json:"raw"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "bad JSON")
		return
	}
	id := r.PathValue("id")
	var line string
	switch {
	case req.Preset != "" && req.Raw != "":
		httpError(w, http.StatusBadRequest, "send preset or raw, not both")
		return
	case req.Preset != "":
		var err error
		if line, err = gateway.BuildPreset(req.Preset, id, time.Now()); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
	case strings.TrimSpace(req.Raw) != "":
		line = strings.TrimSpace(req.Raw)
	default:
		httpError(w, http.StatusBadRequest, "preset or raw required")
		return
	}
	if err := s.Hub.Send(id, line); err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, gateway.ErrNotConnected) {
			code = http.StatusConflict
		}
		httpError(w, code, err.Error())
		return
	}
	writeJSON(w, map[string]string{"sent": line})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
