package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gotenberg-manager/internal/config"
	"gotenberg-manager/internal/database"
	"gotenberg-manager/internal/handlers"
	"gotenberg-manager/internal/middleware"
	"gotenberg-manager/internal/services"
	"gotenberg-manager/internal/tyk"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("❌ Config error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to database
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("❌ Database error: %v", err)
	}
	defer db.Close()

	// Run migrations
	execPath, _ := os.Executable()
	migrationsDir := filepath.Join(filepath.Dir(execPath), "migrations")
	// Fallback for development
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		migrationsDir = "migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		log.Fatalf("❌ Migration error: %v", err)
	}

	// Initialize Tyk client
	tykClient := tyk.NewClient(cfg.TykURL, cfg.TykAdminKey)

	// Initialize services
	clientSvc := services.NewClientService(db, tykClient)
	usageSvc := services.NewUsageService(db)
	healthSvc := services.NewHealthService(db, cfg.GotenbergURL, cfg.HealthCheckInterval)

	// Start background health checker
	healthSvc.Start(ctx)

	// Initialize handlers
	apiHandler := handlers.NewAPIHandler(clientSvc, usageSvc)
	healthHandler := handlers.NewHealthHandler(healthSvc, db)

	// Resolve templates directory
	templateDir := filepath.Join(filepath.Dir(execPath), "web", "templates")
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		templateDir = filepath.Join("web", "templates")
	}
	staticDir := filepath.Join(filepath.Dir(execPath), "web", "static")
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		staticDir = filepath.Join("web", "static")
	}

	dashboardHandler := handlers.NewDashboardHandler(clientSvc, usageSvc, healthSvc, db, templateDir)

	// Portal handler (client-facing)
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		sessionSecret = "default-session-secret-change-me"
	}
	portalHandler := handlers.NewPortalHandler(clientSvc, usageSvc, cfg.GotenbergURL, sessionSecret, templateDir)

	// Setup router
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger)

	// Health endpoint (public)
	r.Get("/health", healthHandler.GetHealth)

	// Static files
	fileServer := http.FileServer(http.Dir(staticDir))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Dashboard (public web UI — admin)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusMovedPermanently)
	})
	r.Get("/dashboard", dashboardHandler.Dashboard)
	r.Get("/dashboard/clients", dashboardHandler.ClientList)
	r.Get("/dashboard/clients/new", dashboardHandler.ClientForm)
	r.Post("/dashboard/clients/new", dashboardHandler.ClientFormSubmit)
	r.Get("/dashboard/clients/{id}", dashboardHandler.ClientDetail)
	r.Post("/dashboard/clients/{id}/delete", dashboardHandler.ClientDelete)
	r.Get("/dashboard/health", dashboardHandler.HealthPage)

	// REST API (admin token protected)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg.AdminToken))

		r.Get("/clients", apiHandler.ListClients)
		r.Post("/clients", apiHandler.CreateClient)
		r.Get("/clients/{id}", apiHandler.GetClient)
		r.Put("/clients/{id}", apiHandler.UpdateClient)
		r.Delete("/clients/{id}", apiHandler.DeleteClient)
		r.Post("/clients/{id}/rotate-key", apiHandler.RotateKey)
		r.Get("/clients/{id}/usage", apiHandler.GetClientUsage)
		r.Get("/usage/summary", apiHandler.GetUsageSummary)
	})

	// Client Portal (public login + session-protected pages)
	r.Get("/portal/login", portalHandler.LoginPage)
	r.Post("/portal/login", portalHandler.LoginSubmit)
	r.Route("/portal", func(r chi.Router) {
		r.Use(middleware.ClientAuth(sessionSecret))

		r.Get("/", portalHandler.Dashboard)
		r.Get("/generate", portalHandler.GenerateForm)
		r.Post("/generate", portalHandler.GenerateSubmit)
		r.Get("/subscription", portalHandler.Subscription)
		r.Post("/logout", portalHandler.Logout)
	})

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("🛑 Shutting down gracefully...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("⚠️  Shutdown error: %v", err)
		}
	}()

	log.Printf("🚀 Gotenberg Manager listening on :%s", cfg.Port)
	log.Printf("📊 Dashboard: http://localhost:%s/dashboard", cfg.Port)
	log.Printf("🔌 API:       http://localhost:%s/api/", cfg.Port)
	log.Printf("🏥 Health:    http://localhost:%s/health", cfg.Port)

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("❌ Server error: %v", err)
	}

	log.Println("👋 Server stopped")
}
