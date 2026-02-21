// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// HTTP API for statistics

package api

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/mlapointe/ipxtransporter/internal/config"
	"github.com/mlapointe/ipxtransporter/internal/relay"
	"github.com/mlapointe/ipxtransporter/internal/stats"
)

//go:embed templates/stats.tmpl
var templatesFS embed.FS

type API struct {
	statsFunc func() stats.Stats
	tmpl      *template.Template
	srv       *relay.Server
	adminUser string
	adminPass string
	cfg       *config.Config
}

func NewAPI(srv *relay.Server, cfg *config.Config) *API {
	tmpl, err := template.ParseFS(templatesFS, "templates/stats.tmpl")
	if err != nil {
		log.Printf("Warning: failed to parse templates/stats.tmpl: %v", err)
	}

	return &API{
		srv:       srv,
		statsFunc: srv.CollectStats,
		tmpl:      tmpl,
		cfg:       cfg,
	}
}

func (a *API) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/stats.html", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/stats", a.statsHandler)
	mux.HandleFunc("/stats.html", a.statsHandler)
	mux.HandleFunc("/api/action", a.actionHandler)
	mux.HandleFunc("/api/sort", a.sortHandler)
	mux.HandleFunc("/api/demo", a.demoHandler)
	mux.HandleFunc("/api/login", a.loginHandler)
	mux.HandleFunc("/api/config", a.configHandler)

	log.Printf("HTTP API listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (a *API) statsHandler(w http.ResponseWriter, r *http.Request) {
	s := a.statsFunc()

	if strings.HasSuffix(r.URL.Path, ".html") {
		if a.tmpl == nil {
			http.Error(w, "Template not loaded", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if err := a.tmpl.Execute(w, s); err != nil {
			log.Printf("Template execute error: %v", err)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	}
}

func (a *API) sortHandler(w http.ResponseWriter, r *http.Request) {
	field := r.URL.Query().Get("field")
	if field != "" {
		a.srv.SetSortField(field)
	}
	w.WriteHeader(http.StatusOK)
}

func (a *API) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.User == a.cfg.AdminUser && req.Pass == a.cfg.AdminPass {
		// In a real app, use a session/JWT. For now, just return success.
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	} else {
		json.NewEncoder(w).Encode(map[string]any{"success": false})
	}
}

func (a *API) actionHandler(w http.ResponseWriter, r *http.Request) {
	// Simple auth check (mock)
	// if !a.isAuthorized(r) { http.Error(w, "Unauthorized", http.StatusUnauthorized); return }

	var req struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		IP     string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	switch req.Action {
	case "disconnect":
		a.srv.DisconnectPeer(req.ID)
	case "ban":
		a.srv.BanPeer(req.ID, req.IP)
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (a *API) demoHandler(w http.ResponseWriter, r *http.Request) {
	var req stats.DemoProps
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	a.srv.UpdateDemoProps(req.PacketRate, req.DropRate, req.ErrorRate, req.NumPeers)
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (a *API) configHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdminPass string `json:"admin_pass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.AdminPass != "" {
		a.cfg.AdminPass = req.AdminPass
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}
