package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gotenberg-manager/internal/database"
	"gotenberg-manager/internal/models"
	"gotenberg-manager/internal/tyk"

	"golang.org/x/crypto/bcrypt"
)

type ClientService struct {
	db  *database.DB
	tyk *tyk.Client
}

func NewClientService(db *database.DB, tykClient *tyk.Client) *ClientService {
	return &ClientService{db: db, tyk: tykClient}
}

func (s *ClientService) Create(ctx context.Context, req models.CreateClientRequest) (*models.Client, error) {
	// Set default plan limits
	if req.Plan == "" {
		req.Plan = "free"
	}
	if req.MonthlyLimit <= 0 {
		if limit, ok := models.PlanLimits[req.Plan]; ok {
			req.MonthlyLimit = limit
		} else {
			req.MonthlyLimit = 100
		}
	}

	// Generate API key
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash password if provided
	var passwordHash string
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		passwordHash = string(hash)
	}

	// Try to create Tyk key (non-blocking if Tyk is unavailable)
	var tykKeyID string
	tykResp, err := s.tyk.CreateKey(100, 60, req.MonthlyLimit)
	if err != nil {
		// Log but don't fail — Tyk might not be available in dev
		fmt.Printf("⚠️  Could not create Tyk key (continuing without): %v\n", err)
	} else {
		tykKeyID = tykResp.Key
	}

	// Insert into database
	var client models.Client
	err = s.db.Pool.QueryRow(ctx, `
		INSERT INTO clients (name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
	`, req.Name, req.Email, apiKey, tykKeyID, passwordHash, req.Plan, req.MonthlyLimit).Scan(
		&client.ID, &client.Name, &client.Email, &client.APIKey,
		&client.TykKeyID, &client.PasswordHash, &client.Plan, &client.MonthlyLimit,
		&client.IsActive, &client.CreatedAt, &client.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &client, nil
}

func (s *ClientService) List(ctx context.Context) ([]models.Client, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
		FROM clients ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list clients: %w", err)
	}
	defer rows.Close()

	var clients []models.Client
	for rows.Next() {
		var c models.Client
		err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
			&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan client: %w", err)
		}
		clients = append(clients, c)
	}
	return clients, nil
}

func (s *ClientService) GetByID(ctx context.Context, id string) (*models.Client, error) {
	var c models.Client
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
		FROM clients WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
		&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}
	return &c, nil
}

func (s *ClientService) GetByAPIKey(ctx context.Context, apiKey string) (*models.Client, error) {
	var c models.Client
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
		FROM clients WHERE api_key = $1 AND is_active = true
	`, apiKey).Scan(&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
		&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}
	return &c, nil
}

func (s *ClientService) Update(ctx context.Context, id string, req models.UpdateClientRequest) (*models.Client, error) {
	var c models.Client
	err := s.db.Pool.QueryRow(ctx, `
		UPDATE clients SET name = $2, email = $3, plan = $4, monthly_limit = $5, is_active = $6, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
	`, id, req.Name, req.Email, req.Plan, req.MonthlyLimit, req.IsActive).Scan(
		&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
		&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update client: %w", err)
	}
	return &c, nil
}

func (s *ClientService) Delete(ctx context.Context, id string) error {
	// Get client to find Tyk key
	client, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete Tyk key if exists
	if client.TykKeyID != "" {
		if err := s.tyk.DeleteKey(client.TykKeyID); err != nil {
			fmt.Printf("⚠️  Could not delete Tyk key: %v\n", err)
		}
	}

	_, err = s.db.Pool.Exec(ctx, "DELETE FROM clients WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete client: %w", err)
	}
	return nil
}

func (s *ClientService) RotateKey(ctx context.Context, id string) (*models.Client, error) {
	newKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate new API key: %w", err)
	}

	var c models.Client
	err = s.db.Pool.QueryRow(ctx, `
		UPDATE clients SET api_key = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
	`, id, newKey).Scan(
		&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
		&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to rotate key: %w", err)
	}
	return &c, nil
}

func (s *ClientService) Count(ctx context.Context) (total int, active int, err error) {
	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM clients").Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM clients WHERE is_active = true").Scan(&active)
	return total, active, err
}

func (s *ClientService) GetRecent(ctx context.Context, limit int) ([]models.Client, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
		FROM clients ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []models.Client
	for rows.Next() {
		var c models.Client
		err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
			&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, nil
}

// Authenticate validates email+password and returns the client if valid.
func (s *ClientService) Authenticate(ctx context.Context, email, password string) (*models.Client, error) {
	var c models.Client
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active, created_at, updated_at
		FROM clients WHERE email = $1 AND is_active = true
	`, email).Scan(&c.ID, &c.Name, &c.Email, &c.APIKey, &c.TykKeyID,
		&c.PasswordHash, &c.Plan, &c.MonthlyLimit, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if c.PasswordHash == "" {
		return nil, fmt.Errorf("password not set for this account")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &c, nil
}

// SetPassword sets or updates a client's password.
func (s *ClientService) SetPassword(ctx context.Context, id, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = s.db.Pool.Exec(ctx, `UPDATE clients SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, string(hash))
	if err != nil {
		return fmt.Errorf("failed to set password: %w", err)
	}
	return nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gm_" + hex.EncodeToString(b), nil
}

// FormatDate is a helper for templates
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}
