package api

import "net/http"

// RegisterRoutes attaches all routes to the mux.
func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /search", h.search)
	mux.HandleFunc("GET /health", h.health)
}