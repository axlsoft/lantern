package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/db"
	"github.com/axlsoft/lantern/internal/handler"
	"github.com/axlsoft/lantern/internal/mailer"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		logger.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database connected")

	m, err := mailer.New(cfg)
	if err != nil {
		logger.Error("init mailer", "err", err)
		os.Exit(1)
	}

	authH := handler.NewAuthHandler(pool, m, cfg)
	orgH := handler.NewOrgHandler(pool, m, cfg)
	apiKeyH := handler.NewAPIKeyHandler(pool, cfg)
	ingestH := handler.NewIngestionHandler(pool, cfg)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	// ── Health ──────────────────────────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"unavailable","error":%q}`, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// ── Auth (public) ────────────────────────────────────────────────────────
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/signup", authH.Signup)
		r.Get("/verify", authH.VerifyEmail)
		r.Post("/login", authH.Login)
		r.Post("/logout", authH.Logout)
		r.Post("/password-reset/request", authH.RequestPasswordReset)
		r.Post("/password-reset/complete", authH.CompletePasswordReset)
	})

	// ── Management API (session-authenticated) ───────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(authH.SessionMiddleware)
		r.Use(handler.RequireSession)

		r.Get("/api/v1/auth/me", authH.Me)

		// Organizations
		r.Post("/api/v1/organizations", orgH.CreateOrg)
		r.Get("/api/v1/organizations/{org_id}", orgH.GetOrg)
		r.Patch("/api/v1/organizations/{org_id}", orgH.UpdateOrg)
		r.Delete("/api/v1/organizations/{org_id}", orgH.DeleteOrg)
		r.Post("/api/v1/organizations/{org_id}/invites", orgH.InviteToOrg)
		r.Post("/api/v1/invites/{token}/accept", orgH.AcceptInvite)

		// Teams
		r.Post("/api/v1/organizations/{org_id}/teams", orgH.CreateTeam)
		r.Get("/api/v1/organizations/{org_id}/teams", orgH.ListTeams)
		r.Get("/api/v1/teams/{team_id}", orgH.GetTeam)
		r.Patch("/api/v1/teams/{team_id}", orgH.UpdateTeam)
		r.Delete("/api/v1/teams/{team_id}", orgH.DeleteTeam)
		r.Post("/api/v1/teams/{team_id}/members", orgH.AddTeamMember)

		// Projects
		r.Post("/api/v1/teams/{team_id}/projects", orgH.CreateProject)
		r.Get("/api/v1/projects/{project_id}", orgH.GetProject)
		r.Patch("/api/v1/projects/{project_id}", orgH.UpdateProject)
		r.Delete("/api/v1/projects/{project_id}", orgH.DeleteProject)

		// API keys (managed via session auth)
		r.Post("/api/v1/projects/{project_id}/api-keys", apiKeyH.CreateAPIKey)
		r.Get("/api/v1/projects/{project_id}/api-keys", apiKeyH.ListAPIKeys)
		r.Post("/api/v1/projects/{project_id}/api-keys/{key_id}/rotate", apiKeyH.RotateAPIKey)
		r.Delete("/api/v1/projects/{project_id}/api-keys/{key_id}", apiKeyH.RevokeAPIKey)
	})

	// ── Ingestion API (API key authenticated) ────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(handler.APIKeyMiddleware(pool, cfg))

		r.Post("/v1/coverage", ingestH.IngestCoverage)
		r.Post("/v1/runs", ingestH.CreateRun)
		r.Patch("/v1/runs/{run_id}", ingestH.UpdateRun)
		r.Post("/v1/runs/{run_id}/tests", ingestH.RegisterTests)
		r.Patch("/v1/runs/{run_id}/tests/{test_id}", ingestH.UpdateTest)
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("collector listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
