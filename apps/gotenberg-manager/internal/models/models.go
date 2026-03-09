package models

import "time"

// --- Entities ---

type Client struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	APIKey       string    `json:"api_key"`
	TykKeyID     string    `json:"tyk_key_id,omitempty"`
	PasswordHash string    `json:"-"`
	Plan         string    `json:"plan"`
	MonthlyLimit int       `json:"monthly_limit"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UsageRecord struct {
	ID             int64     `json:"id"`
	ClientID       string    `json:"client_id"`
	Endpoint       string    `json:"endpoint"`
	StatusCode     int       `json:"status_code"`
	ResponseTimeMs int       `json:"response_time_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

type HealthCheck struct {
	ID             int64     `json:"id"`
	Service        string    `json:"service"`
	Status         string    `json:"status"`
	ResponseTimeMs int       `json:"response_time_ms"`
	Details        string    `json:"details"`
	CheckedAt      time.Time `json:"checked_at"`
}

// --- Request DTOs ---

type CreateClientRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password,omitempty"`
	Plan         string `json:"plan"`
	MonthlyLimit int    `json:"monthly_limit"`
}

type UpdateClientRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Plan         string `json:"plan"`
	MonthlyLimit int    `json:"monthly_limit"`
	IsActive     bool   `json:"is_active"`
}

// --- Response DTOs ---

type HealthResponse struct {
	Status    string            `json:"status"`
	App       string            `json:"app"`
	Gotenberg HealthServiceInfo `json:"gotenberg"`
	Database  string            `json:"database"`
	Timestamp time.Time         `json:"timestamp"`
}

type HealthServiceInfo struct {
	Status         string    `json:"status"`
	ResponseTimeMs int       `json:"response_time_ms"`
	LastChecked    time.Time `json:"last_checked"`
}

type UsageStats struct {
	ClientID     string `json:"client_id,omitempty"`
	ClientName   string `json:"client_name,omitempty"`
	Today        int    `json:"today"`
	ThisMonth    int    `json:"this_month"`
	Total        int    `json:"total"`
	MonthlyLimit int    `json:"monthly_limit,omitempty"`
}

type UsageSummary struct {
	TotalClients  int          `json:"total_clients"`
	ActiveClients int          `json:"active_clients"`
	PDFsToday     int          `json:"pdfs_today"`
	PDFsThisMonth int          `json:"pdfs_this_month"`
	PDFsTotal     int          `json:"pdfs_total"`
	TopClients    []UsageStats `json:"top_clients"`
}

// --- Dashboard View Models ---

type DashboardData struct {
	Summary       UsageSummary
	Health        HealthResponse
	RecentClients []Client
}

type ClientDetailData struct {
	Client Client
	Usage  UsageStats
	Recent []UsageRecord
}

type ClientListData struct {
	Clients []Client
	Total   int
}

// Plan presets
var PlanLimits = map[string]int{
	"free":       100,
	"starter":    1000,
	"pro":        10000,
	"enterprise": 100000,
}

// --- Portal View Models ---

type PortalDashboardData struct {
	Client       Client
	Usage        UsageStats
	RecentUsage  []UsageRecord
	QuotaPercent int
}

type PortalGenerateData struct {
	Client    Client
	QuotaLeft int
	Error     string
	Success   bool
}

type PortalSubscriptionData struct {
	Client       Client
	Usage        UsageStats
	QuotaPercent int
	Plans        map[string]int
}

type PortalLoginData struct {
	Error string
}
