package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"gotenberg-manager/internal/database"
	"gotenberg-manager/internal/models"
	"gotenberg-manager/internal/services"

	"github.com/go-chi/chi/v5"
)

type DashboardHandler struct {
	clientSvc *services.ClientService
	usageSvc  *services.UsageService
	healthSvc *services.HealthService
	db        *database.DB
	templates *template.Template
}

func NewDashboardHandler(
	clientSvc *services.ClientService,
	usageSvc *services.UsageService,
	healthSvc *services.HealthService,
	db *database.DB,
	templateDir string,
) *DashboardHandler {
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("02 Jan 2006 15:04")
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"statusColor": func(status string) string {
			switch status {
			case "healthy":
				return "status-healthy"
			case "degraded":
				return "status-degraded"
			default:
				return "status-unhealthy"
			}
		},
		"planBadge": func(plan string) string {
			switch plan {
			case "enterprise":
				return "badge-enterprise"
			case "pro":
				return "badge-pro"
			case "starter":
				return "badge-starter"
			default:
				return "badge-free"
			}
		},
		"percentage": func(used, limit int) int {
			if limit <= 0 {
				return 0
			}
			p := (used * 100) / limit
			if p > 100 {
				return 100
			}
			return p
		},
	}

	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templateDir, "*.html")))

	return &DashboardHandler{
		clientSvc: clientSvc,
		usageSvc:  usageSvc,
		healthSvc: healthSvc,
		db:        db,
		templates: tmpl,
	}
}

func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	summary, _ := h.usageSvc.GetSummary(ctx)
	health := h.healthSvc.GetFullHealth(ctx, h.db)
	recentClients, _ := h.clientSvc.GetRecent(ctx, 5)
	if recentClients == nil {
		recentClients = []models.Client{}
	}

	data := models.DashboardData{
		Summary:       *summary,
		Health:        health,
		RecentClients: recentClients,
	}

	h.render(w, "dashboard.html", data)
}

func (h *DashboardHandler) ClientList(w http.ResponseWriter, r *http.Request) {
	clients, _ := h.clientSvc.List(r.Context())
	if clients == nil {
		clients = []models.Client{}
	}

	data := models.ClientListData{
		Clients: clients,
		Total:   len(clients),
	}
	h.render(w, "clients.html", data)
}

func (h *DashboardHandler) ClientDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	client, err := h.clientSvc.GetByID(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/dashboard/clients", http.StatusSeeOther)
		return
	}

	stats, _ := h.usageSvc.GetClientStats(ctx, id)
	recent, _ := h.usageSvc.GetRecentRecords(ctx, id, 20)
	if recent == nil {
		recent = []models.UsageRecord{}
	}
	if stats == nil {
		stats = &models.UsageStats{}
	}
	stats.MonthlyLimit = client.MonthlyLimit

	data := models.ClientDetailData{
		Client: *client,
		Usage:  *stats,
		Recent: recent,
	}
	h.render(w, "client_detail.html", data)
}

func (h *DashboardHandler) ClientForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "client_form.html", nil)
}

func (h *DashboardHandler) ClientFormSubmit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	plan := r.FormValue("plan")
	monthlyLimit := 100
	if limit, ok := models.PlanLimits[plan]; ok {
		monthlyLimit = limit
	}

	req := models.CreateClientRequest{
		Name:         r.FormValue("name"),
		Email:        r.FormValue("email"),
		Plan:         plan,
		MonthlyLimit: monthlyLimit,
	}

	_, err := h.clientSvc.Create(r.Context(), req)
	if err != nil {
		log.Printf("Error creating client: %v", err)
		http.Redirect(w, r, "/dashboard/clients/new?error=1", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/clients", http.StatusSeeOther)
}

func (h *DashboardHandler) ClientDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.clientSvc.Delete(r.Context(), id)
	http.Redirect(w, r, "/dashboard/clients", http.StatusSeeOther)
}

func (h *DashboardHandler) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
