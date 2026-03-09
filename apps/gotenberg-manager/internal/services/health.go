package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"gotenberg-manager/internal/database"
	"gotenberg-manager/internal/models"
)

type HealthService struct {
	db           *database.DB
	gotenbergURL string
	interval     time.Duration
	http         *http.Client

	mu     sync.RWMutex
	latest models.HealthServiceInfo
}

func NewHealthService(db *database.DB, gotenbergURL string, intervalSecs int) *HealthService {
	return &HealthService{
		db:           db,
		gotenbergURL: gotenbergURL,
		interval:     time.Duration(intervalSecs) * time.Second,
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
		latest: models.HealthServiceInfo{
			Status: "unknown",
		},
	}
}

// Start begins the background health check loop
func (s *HealthService) Start(ctx context.Context) {
	// Initial check
	s.check(ctx)

	ticker := time.NewTicker(s.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.check(ctx)
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
	log.Printf("🏥 Health checker started (every %s)", s.interval)
}

func (s *HealthService) check(ctx context.Context) {
	start := time.Now()
	status := "healthy"
	details := ""

	resp, err := s.http.Get(s.gotenbergURL + "/health")
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		status = "unhealthy"
		details = err.Error()
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			status = "degraded"
			details = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	// Update in-memory latest
	s.mu.Lock()
	s.latest = models.HealthServiceInfo{
		Status:         status,
		ResponseTimeMs: int(elapsed),
		LastChecked:    time.Now(),
	}
	s.mu.Unlock()

	// Store in database
	_, err = s.db.Pool.Exec(ctx, `
		INSERT INTO health_checks (service, status, response_time_ms, details)
		VALUES ($1, $2, $3, $4)
	`, "gotenberg", status, int(elapsed), details)
	if err != nil {
		log.Printf("⚠️  Failed to store health check: %v", err)
	}

	if status != "healthy" {
		log.Printf("⚠️  Gotenberg health: %s (%s)", status, details)
	}
}

// GetStatus returns the current health status
func (s *HealthService) GetStatus() models.HealthServiceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

// GetFullHealth returns the complete health response
func (s *HealthService) GetFullHealth(ctx context.Context, db *database.DB) models.HealthResponse {
	gotenberg := s.GetStatus()

	dbStatus := "healthy"
	if err := db.Ping(ctx); err != nil {
		dbStatus = "unhealthy"
	}

	overall := "healthy"
	if gotenberg.Status != "healthy" || dbStatus != "healthy" {
		overall = "degraded"
	}
	if gotenberg.Status == "unhealthy" && dbStatus == "unhealthy" {
		overall = "unhealthy"
	}

	return models.HealthResponse{
		Status:    overall,
		App:       "healthy",
		Gotenberg: gotenberg,
		Database:  dbStatus,
		Timestamp: time.Now(),
	}
}

// GetHistory returns recent health check entries
func (s *HealthService) GetHistory(ctx context.Context, limit int) ([]models.HealthCheck, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, service, status, response_time_ms, details, checked_at
		FROM health_checks ORDER BY checked_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []models.HealthCheck
	for rows.Next() {
		var h models.HealthCheck
		err := rows.Scan(&h.ID, &h.Service, &h.Status, &h.ResponseTimeMs, &h.Details, &h.CheckedAt)
		if err != nil {
			return nil, err
		}
		checks = append(checks, h)
	}
	return checks, nil
}
