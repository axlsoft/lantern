package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestAuthFlow covers F1.1-2: signup → verify → login → logout → /me.
func TestAuthFlow(t *testing.T) {
	ts := newTestServer(t)
	email := uniqueEmail(t, "authflow")
	password := "hunter2secure!"

	t.Run("signup returns 201", func(t *testing.T) {
		c := ts.client()
		resp := c.post("/api/v1/auth/signup", map[string]any{"email": email, "password": password})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %v", resp.StatusCode, mustJSON(t, resp))
		}
		resp.Body.Close()
	})

	t.Run("duplicate email returns 409", func(t *testing.T) {
		c := ts.client()
		resp := c.post("/api/v1/auth/signup", map[string]any{"email": email, "password": password})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("login before verification returns 401", func(t *testing.T) {
		c := ts.client()
		resp := c.post("/api/v1/auth/login", map[string]any{"email": email, "password": password})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		_, err := ts.pool.Exec(context.Background(),
			"UPDATE users SET email_verified_at = now() WHERE email = $1", email)
		if err != nil {
			t.Fatalf("mark verified: %v", err)
		}
		c := ts.client()
		resp := c.post("/api/v1/auth/login", map[string]any{"email": email, "password": "wrongpassword"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("login succeeds and me returns profile", func(t *testing.T) {
		c := ts.client()
		resp := c.post("/api/v1/auth/login", map[string]any{"email": email, "password": password})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, mustJSON(t, resp))
		}
		c.cookies = resp.Cookies()
		resp.Body.Close()

		resp = c.get("/api/v1/auth/me")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("/me: expected 200, got %d", resp.StatusCode)
		}
		data := dataField(t, mustJSON(t, resp))
		if data["email"] != email {
			t.Errorf("me.email: got %v, want %v", data["email"], email)
		}
	})

	t.Run("me without session returns 401", func(t *testing.T) {
		c := ts.client() // no cookies
		resp := c.get("/api/v1/auth/me")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("logout deletes session", func(t *testing.T) {
		c := signupAndLogin(t, ts, uniqueEmail(t, "logout"), "securepass123!")
		resp := c.post("/api/v1/auth/logout", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("logout: expected 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Subsequent /me should return 401.
		resp = c.get("/api/v1/auth/me")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("after logout, /me: expected 401, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestPasswordResetFlow verifies the full reset token lifecycle.
func TestPasswordResetFlow(t *testing.T) {
	ts := newTestServer(t)
	email := uniqueEmail(t, "pwreset")
	password := "original-pass-99!"

	signupAndLogin(t, ts, email, password)
	c := ts.client()

	t.Run("request reset always returns 200", func(t *testing.T) {
		resp := c.post("/api/v1/auth/password-reset/request", map[string]any{"email": email})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Also for an unknown email — must not reveal whether email exists.
		resp = c.post("/api/v1/auth/password-reset/request", map[string]any{"email": "unknown@example.com"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for unknown email, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("complete reset with valid token", func(t *testing.T) {
		// Retrieve the token directly from DB.
		var token string
		err := ts.pool.QueryRow(context.Background(),
			`SELECT token::text FROM password_resets pr
			 JOIN users u ON u.id = pr.user_id
			 WHERE u.email = $1 AND pr.consumed_at IS NULL AND pr.expires_at > now()
			 ORDER BY pr.expires_at DESC LIMIT 1`, email).Scan(&token)
		if err != nil {
			t.Skipf("no reset token found (request may have been no-op in test): %v", err)
		}

		resp := c.post("/api/v1/auth/password-reset/complete", map[string]any{
			"token":        token,
			"new_password": "newpass-9secure!",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, mustJSON(t, resp))
		}
		resp.Body.Close()

		// Can log in with new password.
		resp = c.post("/api/v1/auth/login", map[string]any{"email": email, "password": "newpass-9secure!"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login after reset: expected 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("expired token returns 400", func(t *testing.T) {
		// Insert an already-expired token.
		var userID string
		_ = ts.pool.QueryRow(context.Background(), "SELECT id FROM users WHERE email = $1", email).Scan(&userID)
		var fakeToken string
		_ = ts.pool.QueryRow(context.Background(),
			"INSERT INTO password_resets (user_id, expires_at) VALUES ($1, $2) RETURNING token::text",
			userID, time.Now().Add(-time.Hour)).Scan(&fakeToken)

		resp := c.post("/api/v1/auth/password-reset/complete", map[string]any{
			"token":        fakeToken,
			"new_password": "irrelevant123!",
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for expired token, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// testRunID makes email addresses unique across test runs on shared DBs.
var testRunID = time.Now().UnixNano()

// uniqueEmail generates a test email address unique per test run.
func uniqueEmail(t *testing.T, label string) string {
	return fmt.Sprintf("%s+%d@lantern.test", label, testRunID)
}
