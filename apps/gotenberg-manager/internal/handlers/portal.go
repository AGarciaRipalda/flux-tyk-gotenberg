package handlers

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gotenberg-manager/internal/middleware"
	"gotenberg-manager/internal/models"
	"gotenberg-manager/internal/services"
)

type PortalHandler struct {
	clientSvc     *services.ClientService
	usageSvc      *services.UsageService
	gotenbergURL  string
	sessionSecret string
	templates     map[string]*template.Template
}

func NewPortalHandler(
	clientSvc *services.ClientService,
	usageSvc *services.UsageService,
	gotenbergURL string,
	sessionSecret string,
	templateDir string,
) *PortalHandler {
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("02 Jan 2006 15:04")
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
		"subtract": func(a, b int) int {
			r := a - b
			if r < 0 {
				return 0
			}
			return r
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
		"upper": strings.ToUpper,
		"statusCode": func(code int) string {
			if code >= 200 && code < 300 {
				return "status-success"
			} else if code >= 400 {
				return "status-error"
			}
			return ""
		},
		"formatNumber": func(n int) string {
			if n >= 1000 {
				return fmt.Sprintf("%dk", n/1000)
			}
			return fmt.Sprintf("%d", n)
		},
	}

	templates := make(map[string]*template.Template)
	portalPages := []string{
		"portal_login.html",
		"portal_dashboard.html",
		"portal_generate.html",
		"portal_subscription.html",
	}

	portalLayoutPath := filepath.Join(templateDir, "portal_layout.html")

	// Login page uses its own template (no layout)
	loginPath := filepath.Join(templateDir, "portal_login.html")
	tmpl := template.Must(template.New("portal_login.html").Funcs(funcMap).ParseFiles(loginPath))
	templates["portal_login.html"] = tmpl

	// Portal pages use the portal layout
	for _, page := range portalPages {
		if page == "portal_login.html" {
			continue // already parsed above
		}
		pagePath := filepath.Join(templateDir, page)
		tmpl := template.Must(template.New(page).Funcs(funcMap).ParseFiles(portalLayoutPath, pagePath))
		templates[page] = tmpl
	}

	return &PortalHandler{
		clientSvc:     clientSvc,
		usageSvc:      usageSvc,
		gotenbergURL:  gotenbergURL,
		sessionSecret: sessionSecret,
		templates:     templates,
	}
}

// ---- Login / Logout ----

func (h *PortalHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	errorMsg := r.URL.Query().Get("error")
	h.render(w, "portal_login.html", models.PortalLoginData{Error: errorMsg})
}

func (h *PortalHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	email := r.FormValue("email")
	password := r.FormValue("password")

	client, err := h.clientSvc.Authenticate(r.Context(), email, password)
	if err != nil {
		http.Redirect(w, r, "/portal/login?error=Invalid+email+or+password", http.StatusSeeOther)
		return
	}

	middleware.SetSessionCookie(w, client.ID, h.sessionSecret)
	http.Redirect(w, r, "/portal", http.StatusSeeOther)
}

func (h *PortalHandler) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.ClearSessionCookie(w)
	http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
}

// ---- Dashboard ----

func (h *PortalHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientID := middleware.ClientIDFromContext(ctx)

	client, err := h.clientSvc.GetByID(ctx, clientID)
	if err != nil {
		http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
		return
	}

	stats, _ := h.usageSvc.GetClientStats(ctx, clientID)
	if stats == nil {
		stats = &models.UsageStats{}
	}
	stats.MonthlyLimit = client.MonthlyLimit

	recent, _ := h.usageSvc.GetRecentRecords(ctx, clientID, 10)
	if recent == nil {
		recent = []models.UsageRecord{}
	}

	quotaPercent := 0
	if client.MonthlyLimit > 0 {
		quotaPercent = (stats.ThisMonth * 100) / client.MonthlyLimit
		if quotaPercent > 100 {
			quotaPercent = 100
		}
	}

	data := models.PortalDashboardData{
		Client:       *client,
		Usage:        *stats,
		RecentUsage:  recent,
		QuotaPercent: quotaPercent,
	}

	h.render(w, "portal_dashboard.html", data)
}

// ---- Generate PDF ----

func (h *PortalHandler) GenerateForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientID := middleware.ClientIDFromContext(ctx)

	client, err := h.clientSvc.GetByID(ctx, clientID)
	if err != nil {
		http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
		return
	}

	stats, _ := h.usageSvc.GetClientStats(ctx, clientID)
	quotaLeft := client.MonthlyLimit
	if stats != nil {
		quotaLeft = client.MonthlyLimit - stats.ThisMonth
		if quotaLeft < 0 {
			quotaLeft = 0
		}
	}

	msg := r.URL.Query().Get("error")
	success := r.URL.Query().Get("success") == "1"

	data := models.PortalGenerateData{
		Client:    *client,
		QuotaLeft: quotaLeft,
		Error:     msg,
		Success:   success,
	}

	h.render(w, "portal_generate.html", data)
}

func (h *PortalHandler) GenerateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientID := middleware.ClientIDFromContext(ctx)

	client, err := h.clientSvc.GetByID(ctx, clientID)
	if err != nil {
		http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
		return
	}

	// Check quota
	withinLimit, _, err := h.usageSvc.CheckLimit(ctx, clientID, client.MonthlyLimit)
	if err != nil || !withinLimit {
		http.Redirect(w, r, "/portal/generate?error=Monthly+quota+exceeded", http.StatusSeeOther)
		return
	}

	// Parse the form (32 MB max for file uploads)
	r.ParseMultipartForm(32 << 20)
	mode := r.FormValue("mode")

	start := time.Now()
	var gotenbergResp *http.Response
	var endpoint string

	switch mode {
	case "url":
		url := r.FormValue("url")
		if url == "" {
			http.Redirect(w, r, "/portal/generate?error=URL+is+required", http.StatusSeeOther)
			return
		}
		endpoint = "/forms/chromium/convert/url"
		gotenbergResp, err = h.proxyURLConversion(url)

	case "html":
		htmlContent := r.FormValue("html")
		if htmlContent == "" {
			http.Redirect(w, r, "/portal/generate?error=HTML+content+is+required", http.StatusSeeOther)
			return
		}
		endpoint = "/forms/chromium/convert/html"
		gotenbergResp, err = h.proxyHTMLConversion(htmlContent)

	case "file":
		file, header, fileErr := r.FormFile("file")
		if fileErr != nil {
			http.Redirect(w, r, "/portal/generate?error=File+is+required", http.StatusSeeOther)
			return
		}
		defer file.Close()
		endpoint = "/forms/libreoffice/convert"
		gotenbergResp, err = h.proxyFileConversion(file, header)

	default:
		http.Redirect(w, r, "/portal/generate?error=Invalid+conversion+mode", http.StatusSeeOther)
		return
	}

	elapsed := int(time.Since(start).Milliseconds())

	if err != nil {
		log.Printf("PDF generation error: %v", err)
		h.usageSvc.Record(ctx, clientID, endpoint, 500, elapsed)
		http.Redirect(w, r, "/portal/generate?error=PDF+generation+failed:+"+err.Error(), http.StatusSeeOther)
		return
	}
	defer gotenbergResp.Body.Close()

	// Record usage
	h.usageSvc.Record(ctx, clientID, endpoint, gotenbergResp.StatusCode, elapsed)

	if gotenbergResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(gotenbergResp.Body)
		log.Printf("Gotenberg returned %d: %s", gotenbergResp.StatusCode, string(body))
		http.Redirect(w, r, "/portal/generate?error=Gotenberg+returned+error+"+fmt.Sprintf("%d", gotenbergResp.StatusCode), http.StatusSeeOther)
		return
	}

	// Stream PDF back to client
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="generated.pdf"`)
	io.Copy(w, gotenbergResp.Body)
}

// ---- Subscription ----

func (h *PortalHandler) Subscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientID := middleware.ClientIDFromContext(ctx)

	client, err := h.clientSvc.GetByID(ctx, clientID)
	if err != nil {
		http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
		return
	}

	stats, _ := h.usageSvc.GetClientStats(ctx, clientID)
	if stats == nil {
		stats = &models.UsageStats{}
	}
	stats.MonthlyLimit = client.MonthlyLimit

	quotaPercent := 0
	if client.MonthlyLimit > 0 {
		quotaPercent = (stats.ThisMonth * 100) / client.MonthlyLimit
		if quotaPercent > 100 {
			quotaPercent = 100
		}
	}

	data := models.PortalSubscriptionData{
		Client:       *client,
		Usage:        *stats,
		QuotaPercent: quotaPercent,
		Plans:        models.PlanLimits,
	}

	h.render(w, "portal_subscription.html", data)
}

// ---- Proxy helpers ----

func (h *PortalHandler) proxyURLConversion(url string) (*http.Response, error) {
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	writer.WriteField("url", url)
	writer.Close()

	req, err := http.NewRequest("POST", h.gotenbergURL+"/forms/chromium/convert/url", strings.NewReader(body.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return http.DefaultClient.Do(req)
}

func (h *PortalHandler) proxyHTMLConversion(htmlContent string) (*http.Response, error) {
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, err
	}
	part.Write([]byte(htmlContent))
	writer.Close()

	req, err := http.NewRequest("POST", h.gotenbergURL+"/forms/chromium/convert/html", strings.NewReader(body.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return http.DefaultClient.Do(req)
}

func (h *PortalHandler) proxyFileConversion(file multipart.File, header *multipart.FileHeader) (*http.Response, error) {
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", header.Filename)
	if err != nil {
		return nil, err
	}
	io.Copy(part, file)
	writer.Close()

	req, err := http.NewRequest("POST", h.gotenbergURL+"/forms/libreoffice/convert", strings.NewReader(body.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return http.DefaultClient.Do(req)
}

// ---- Render helper ----

func (h *PortalHandler) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, ok := h.templates[name]
	if !ok {
		log.Printf("Portal template not found: %s", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Portal template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
