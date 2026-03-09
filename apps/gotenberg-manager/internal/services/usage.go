package services

import (
	"context"
	"fmt"
	"time"

	"gotenberg-manager/internal/database"
	"gotenberg-manager/internal/models"
)

type UsageService struct {
	db *database.DB
}

func NewUsageService(db *database.DB) *UsageService {
	return &UsageService{db: db}
}

// Record stores a new usage record
func (s *UsageService) Record(ctx context.Context, clientID, endpoint string, statusCode, responseTimeMs int) error {
	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO usage_records (client_id, endpoint, status_code, response_time_ms)
		VALUES ($1, $2, $3, $4)
	`, clientID, endpoint, statusCode, responseTimeMs)
	if err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}
	return nil
}

// GetClientStats returns usage statistics for a specific client
func (s *UsageService) GetClientStats(ctx context.Context, clientID string) (*models.UsageStats, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	stats := &models.UsageStats{ClientID: clientID}

	// Today
	err := s.db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM usage_records WHERE client_id = $1 AND created_at >= $2",
		clientID, startOfDay).Scan(&stats.Today)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily stats: %w", err)
	}

	// This month
	err = s.db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM usage_records WHERE client_id = $1 AND created_at >= $2",
		clientID, startOfMonth).Scan(&stats.ThisMonth)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly stats: %w", err)
	}

	// Total
	err = s.db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM usage_records WHERE client_id = $1",
		clientID).Scan(&stats.Total)
	if err != nil {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	return stats, nil
}

// GetSummary returns a global usage summary
func (s *UsageService) GetSummary(ctx context.Context) (*models.UsageSummary, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	summary := &models.UsageSummary{}

	// Client counts
	s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM clients").Scan(&summary.TotalClients)
	s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM clients WHERE is_active = true").Scan(&summary.ActiveClients)

	// PDF counts
	s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM usage_records WHERE created_at >= $1", startOfDay).Scan(&summary.PDFsToday)
	s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM usage_records WHERE created_at >= $1", startOfMonth).Scan(&summary.PDFsThisMonth)
	s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM usage_records").Scan(&summary.PDFsTotal)

	// Top clients by monthly usage
	rows, err := s.db.Pool.Query(ctx, `
		SELECT c.id, c.name, c.monthly_limit,
			COUNT(*) FILTER (WHERE u.created_at >= $1) AS this_month,
			COUNT(*) AS total
		FROM usage_records u
		JOIN clients c ON c.id = u.client_id
		GROUP BY c.id, c.name, c.monthly_limit
		ORDER BY this_month DESC
		LIMIT 5
	`, startOfMonth)
	if err != nil {
		return summary, nil // Return partial data
	}
	defer rows.Close()

	for rows.Next() {
		var us models.UsageStats
		err := rows.Scan(&us.ClientID, &us.ClientName, &us.MonthlyLimit, &us.ThisMonth, &us.Total)
		if err != nil {
			continue
		}
		summary.TopClients = append(summary.TopClients, us)
	}

	return summary, nil
}

// GetRecentRecords returns recent usage records for a client
func (s *UsageService) GetRecentRecords(ctx context.Context, clientID string, limit int) ([]models.UsageRecord, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, client_id, endpoint, status_code, response_time_ms, created_at
		FROM usage_records WHERE client_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, clientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.UsageRecord
	for rows.Next() {
		var r models.UsageRecord
		err := rows.Scan(&r.ID, &r.ClientID, &r.Endpoint, &r.StatusCode, &r.ResponseTimeMs, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

// CheckLimit returns true if the client is within their monthly limit
func (s *UsageService) CheckLimit(ctx context.Context, clientID string, monthlyLimit int) (bool, int, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var count int
	err := s.db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM usage_records WHERE client_id = $1 AND created_at >= $2",
		clientID, startOfMonth).Scan(&count)
	if err != nil {
		return false, 0, err
	}

	return count < monthlyLimit, count, nil
}
