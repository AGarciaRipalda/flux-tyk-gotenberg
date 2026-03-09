package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"gotenberg-manager/internal/database"
	"gotenberg-manager/internal/services"
)

type HealthHandler struct {
	healthSvc *services.HealthService
	db        *database.DB
}

func NewHealthHandler(healthSvc *services.HealthService, db *database.DB) *HealthHandler {
	return &HealthHandler{healthSvc: healthSvc, db: db}
}

func (h *HealthHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	health := h.healthSvc.GetFullHealth(ctx, h.db)

	w.Header().Set("Content-Type", "application/json")
	if health.Status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(health)
}
