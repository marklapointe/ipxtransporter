// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// HTTP API for statistics

package api

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mlapointe/ipxtransporter/internal/logger"

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
		logger.Error("Warning: failed to parse templates/stats.tmpl: %v", err)
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
	mux.HandleFunc("/api/action", a.withAuth(a.actionHandler))
	mux.HandleFunc("/api/sort", a.sortHandler)
	mux.HandleFunc("/api/demo", a.withAuth(a.demoHandler))
	mux.HandleFunc("/api/login", a.loginHandler)
	mux.HandleFunc("/api/config", a.withAuth(a.configHandler))
	mux.HandleFunc("/api/peers/add", a.withAuth(a.addPeerHandler))

	logger.Info("HTTP API listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (a *API) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(a.cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
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
			logger.Error("Template execute error: %v", err)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(s)
		if err != nil {
			return
		}
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
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user": req.User,
			"exp":  time.Now().Add(24 * time.Hour).Unix(),
		})

		tokenString, err := token.SignedString([]byte(a.cfg.JWTSecret))
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"token":   tokenString,
		})
	} else {
		err := json.NewEncoder(w).Encode(map[string]any{"success": false})
		if err != nil {
			return
		}
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
	err := json.NewEncoder(w).Encode(map[string]any{"success": true})
	if err != nil {
		return
	}
}

func (a *API) demoHandler(w http.ResponseWriter, r *http.Request) {
	var req stats.DemoProps
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	a.srv.UpdateDemoProps(req.PacketRate, req.DropRate, req.ErrorRate, req.NumPeers)
	err := json.NewEncoder(w).Encode(map[string]any{"success": true})
	if err != nil {
		return
	}
}

func (a *API) configHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdminPass         string `json:"admin_pass"`
		MaxChildren       int    `json:"max_children"`
		NetworkKey        string `json:"network_key"`
		RebalanceEnabled  bool   `json:"rebalance_enabled"`
		RebalanceInterval int    `json:"rebalance_interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	a.srv.UpdateConfig(req.AdminPass, req.MaxChildren, req.NetworkKey, req.RebalanceEnabled, req.RebalanceInterval)
	err := json.NewEncoder(w).Encode(map[string]any{"success": true})
	if err != nil {
		return
	}
}

func (a *API) addPeerHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Addr string `json:"addr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.Addr == "" {
		http.Error(w, "Address is required", http.StatusBadRequest)
		return
	}

	a.srv.AddPeer(r.Context(), req.Addr)
	err := json.NewEncoder(w).Encode(map[string]any{"success": true})
	if err != nil {
		return
	}
}
