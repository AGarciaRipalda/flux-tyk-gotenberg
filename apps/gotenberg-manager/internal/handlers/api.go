package handlers

import (
	"encoding/json"
	"net/http"

	"gotenberg-manager/internal/models"
	"gotenberg-manager/internal/services"

	"github.com/go-chi/chi/v5"
)

type APIHandler struct {
	clientSvc *services.ClientService
	usageSvc  *services.UsageService
}

func NewAPIHandler(clientSvc *services.ClientService, usageSvc *services.UsageService) *APIHandler {
	return &APIHandler{clientSvc: clientSvc, usageSvc: usageSvc}
}

// --- Clients ---

func (h *APIHandler) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.clientSvc.List(r.Context())
	if err != nil {
		jsonError(w, "failed to list clients", http.StatusInternalServerError)
		return
	}
	if clients == nil {
		clients = []models.Client{}
	}
	jsonResponse(w, clients)
}

func (h *APIHandler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req models.CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Email == "" {
		jsonError(w, "name and email are required", http.StatusBadRequest)
		return
	}

	client, err := h.clientSvc.Create(r.Context(), req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonResponse(w, client)
}

func (h *APIHandler) GetClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	client, err := h.clientSvc.GetByID(r.Context(), id)
	if err != nil {
		jsonError(w, "client not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, client)
}

func (h *APIHandler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req models.UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client, err := h.clientSvc.Update(r.Context(), id, req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, client)
}

func (h *APIHandler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.clientSvc.Delete(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) RotateKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	client, err := h.clientSvc.RotateKey(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, client)
}

// --- Usage ---

func (h *APIHandler) GetClientUsage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	stats, err := h.usageSvc.GetClientStats(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, stats)
}

func (h *APIHandler) GetUsageSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.usageSvc.GetSummary(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, summary)
}

// --- Helpers ---

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
