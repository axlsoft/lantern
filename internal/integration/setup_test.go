// Package integration contains tests that require a real Postgres database.
// Set LANTERN_DATABASE_URL and LANTERN_API_KEY_PEPPER before running.
// Tests are skipped automatically when the env var is absent.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/handler"
)

// testServer bundles the httptest.Server and a pool for direct DB assertions.
type testServer struct {
	*httptest.Server
	pool *pgxpool.Pool
	cfg  *config.Config
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dbURL := os.Getenv("LANTERN_DATABASE_URL")
	if dbURL == "" {
		t.Skip("LANTERN_DATABASE_URL not set; skipping integration tests")
	}

	pepper := os.Getenv("LANTERN_API_KEY_PEPPER")
	if pepper == "" {
		pepper = "test-pepper-integration"
	}

	cfg := &config.Config{
		HTTPAddr:            ":0",
		ShutdownTimeout:     5 * time.Second,
		MaxRequestBodyBytes: 10 * 1024 * 1024,
		DatabaseURL:         dbURL,
		DBMaxConns:          5,
		DBMinConns:          1,
		DBConnTimeout:       10 * time.Second,
		SessionDuration:     1 * time.Hour,
		APIKeyPepper:        pepper,
		SMTPHost:            "localhost",
		SMTPPort:            1025,
		EmailFrom:           "noreply@lantern.test",
		BaseURL:             "http://localhost",
		Env:                 "test",
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to DB: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DB: %v", err)
	}
	t.Cleanup(pool.Close)

	// Mailer in test mode: no actual SMTP (dev MailHog may not be running).
	m := &noopMailer{}

	authH := handler.NewAuthHandlerWithMailer(pool, m, cfg)
	orgH := handler.NewOrgHandler(pool, m, cfg)
	apiKeyH := handler.NewAPIKeyHandler(pool, cfg)
	ingestH := handler.NewIngestionHandler(pool, cfg)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/signup", authH.Signup)
		r.Get("/verify", authH.VerifyEmail)
		r.Post("/login", authH.Login)
		r.Post("/logout", authH.Logout)
		r.Post("/password-reset/request", authH.RequestPasswordReset)
		r.Post("/password-reset/complete", authH.CompletePasswordReset)
	})

	r.Group(func(r chi.Router) {
		r.Use(authH.SessionMiddleware)
		r.Use(handler.RequireSession)
		r.Get("/api/v1/auth/me", authH.Me)
		r.Post("/api/v1/organizations", orgH.CreateOrg)
		r.Get("/api/v1/organizations/{org_id}", orgH.GetOrg)
		r.Patch("/api/v1/organizations/{org_id}", orgH.UpdateOrg)
		r.Delete("/api/v1/organizations/{org_id}", orgH.DeleteOrg)
		r.Post("/api/v1/organizations/{org_id}/invites", orgH.InviteToOrg)
		r.Post("/api/v1/invites/{token}/accept", orgH.AcceptInvite)
		r.Post("/api/v1/organizations/{org_id}/teams", orgH.CreateTeam)
		r.Get("/api/v1/organizations/{org_id}/teams", orgH.ListTeams)
		r.Get("/api/v1/teams/{team_id}", orgH.GetTeam)
		r.Patch("/api/v1/teams/{team_id}", orgH.UpdateTeam)
		r.Delete("/api/v1/teams/{team_id}", orgH.DeleteTeam)
		r.Post("/api/v1/teams/{team_id}/members", orgH.AddTeamMember)
		r.Post("/api/v1/teams/{team_id}/projects", orgH.CreateProject)
		r.Get("/api/v1/projects/{project_id}", orgH.GetProject)
		r.Patch("/api/v1/projects/{project_id}", orgH.UpdateProject)
		r.Delete("/api/v1/projects/{project_id}", orgH.DeleteProject)
		r.Post("/api/v1/projects/{project_id}/api-keys", apiKeyH.CreateAPIKey)
		r.Get("/api/v1/projects/{project_id}/api-keys", apiKeyH.ListAPIKeys)
		r.Post("/api/v1/projects/{project_id}/api-keys/{key_id}/rotate", apiKeyH.RotateAPIKey)
		r.Delete("/api/v1/projects/{project_id}/api-keys/{key_id}", apiKeyH.RevokeAPIKey)
	})

	r.Group(func(r chi.Router) {
		r.Use(handler.APIKeyMiddleware(pool, cfg))
		r.Post("/v1/coverage", ingestH.IngestCoverage)
		r.Post("/v1/runs", ingestH.CreateRun)
		r.Patch("/v1/runs/{run_id}", ingestH.UpdateRun)
		r.Post("/v1/runs/{run_id}/tests", ingestH.RegisterTests)
		r.Patch("/v1/runs/{run_id}/tests/{test_id}", ingestH.UpdateTest)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testServer{Server: srv, pool: pool, cfg: cfg}
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

type client struct {
	base    string
	http    *http.Client
	cookies []*http.Cookie
	apiKey  string
}

func (ts *testServer) client() *client {
	return &client{base: ts.URL, http: ts.Client()}
}

func (c *client) withAPIKey(key string) *client {
	cp := *c
	cp.apiKey = key
	return &cp
}

func (c *client) do(method, path string, body any) *http.Response {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			panic(fmt.Sprintf("marshal request body: %v", err))
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, bodyReader)
	if err != nil {
		panic(fmt.Sprintf("new request: %v", err))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		panic(fmt.Sprintf("do request: %v", err))
	}
	return resp
}

func (c *client) post(path string, body any) *http.Response  { return c.do("POST", path, body) }
func (c *client) get(path string) *http.Response             { return c.do("GET", path, nil) }
func (c *client) patch(path string, body any) *http.Response { return c.do("PATCH", path, body) }
func (c *client) delete(path string) *http.Response          { return c.do("DELETE", path, nil) }

// mustJSON decodes a JSON response body into a map. Closes the body.
func mustJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode JSON response (status %d): %v", resp.StatusCode, err)
	}
	return m
}

func dataField(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	d, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no 'data' object: %v", m)
	}
	return d
}

// signupAndLogin creates a user, marks email verified directly in DB, and logs in.
// Returns the authenticated client (with session cookie set).
func signupAndLogin(t *testing.T, ts *testServer, email, password string) *client {
	t.Helper()
	c := ts.client()

	resp := c.post("/api/v1/auth/signup", map[string]any{"email": email, "password": password})
	if resp.StatusCode != http.StatusCreated {
		mustJSON(t, resp)
		t.Fatalf("signup %s: got %d", email, resp.StatusCode)
	}
	resp.Body.Close()

	// Mark verified directly in DB (no SMTP in test).
	_, err := ts.pool.Exec(context.Background(),
		"UPDATE users SET email_verified_at = now() WHERE email = $1", email)
	if err != nil {
		t.Fatalf("mark email verified: %v", err)
	}

	resp = c.post("/api/v1/auth/login", map[string]any{"email": email, "password": password})
	if resp.StatusCode != http.StatusOK {
		mustJSON(t, resp)
		t.Fatalf("login %s: got %d", email, resp.StatusCode)
	}
	resp.Body.Close()
	c.cookies = resp.Cookies()
	return c
}

// noopMailer silently discards all emails.
type noopMailer struct{}

func (n *noopMailer) SendVerification(to, token string) error                    { return nil }
func (n *noopMailer) SendPasswordReset(to, token string) error                   { return nil }
func (n *noopMailer) SendOrgInvite(to, orgName, token string) error              { return nil }
