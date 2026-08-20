package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"opensearch/internal/orchestrator"
	"opensearch/internal/types"
)

// searchOrchestrator is the interface the handler needs from the orchestrator.
type searchOrchestrator interface {
	Search(ctx context.Context, req orchestrator.Request) (types.Response, error)
}

// Handler holds the orchestrator and handles HTTP requests.
type Handler struct {
	orchestrator searchOrchestrator
}

// NewHandler creates a Handler with the given orchestrator.
func NewHandler(o searchOrchestrator) *Handler {
	return &Handler{orchestrator: o}
}

// searchRequest is the JSON body the agent sends.
type searchRequest struct {
	Query string `json:"query"`
	Intent string `json:"intent"`
	MaxResults int `json:"max_results"`
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	var body searchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Query == "" {
		respondError(w, http.StatusBadRequest, "query must not be empty")
		return
	}

	maxResults := body.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	resp, err := h.orchestrator.Search(r.Context(), orchestrator.Request{
		Query: body.Query,
		AgentIntent: body.Intent,
		MaxResults: maxResults,
	})
	if err != nil {
		if errors.Is(err, orchestrator.ErrInvalidIntent) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respond(w, http.StatusOK, resp)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, map[string]string{"status": "ok"})
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}