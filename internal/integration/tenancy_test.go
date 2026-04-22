package integration_test

import (
	"net/http"
	"testing"
)

// TestTenantIsolation covers F1.1-1: cross-tenant isolation via RLS.
// Two orgs, each with a project; neither can read the other's data.
func TestTenantIsolation(t *testing.T) {
	ts := newTestServer(t)

	// Create two independent users, each gets their own personal org on signup.
	cA := signupAndLogin(t, ts, uniqueEmail(t, "orgA"), "securepassA1!")
	cB := signupAndLogin(t, ts, uniqueEmail(t, "orgB"), "securepassB1!")

	// Each user creates an org (in addition to their personal org).
	respA := cA.post("/api/v1/organizations", map[string]any{"name": "Org Alpha"})
	if respA.StatusCode != http.StatusCreated {
		t.Fatalf("create org A: %d", respA.StatusCode)
	}
	orgA := dataField(t, mustJSON(t, respA))
	orgAID := orgA["id"].(string)

	respB := cB.post("/api/v1/organizations", map[string]any{"name": "Org Beta"})
	if respB.StatusCode != http.StatusCreated {
		t.Fatalf("create org B: %d", respB.StatusCode)
	}
	orgB := dataField(t, mustJSON(t, respB))
	orgBID := orgB["id"].(string)

	// Create a team + project in each org.
	teamA := createTeam(t, cA, orgAID, "Team A")
	teamB := createTeam(t, cB, orgBID, "Team B")
	projA := createProject(t, cA, teamA, "proj-alpha", "Project Alpha")
	projB := createProject(t, cB, teamB, "proj-beta", "Project Beta")

	t.Run("org A cannot read org B", func(t *testing.T) {
		resp := cA.get("/api/v1/organizations/" + orgBID)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("org B cannot read org A", func(t *testing.T) {
		resp := cB.get("/api/v1/organizations/" + orgAID)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("org A cannot read org B's project", func(t *testing.T) {
		resp := cA.get("/api/v1/projects/" + projB)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d (cross-tenant project read must be 404)", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("org B cannot read org A's project", func(t *testing.T) {
		resp := cB.get("/api/v1/projects/" + projA)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("org A cannot delete org B's project", func(t *testing.T) {
		resp := cA.delete("/api/v1/projects/" + projB)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestRBAC verifies role-based access enforcement within a single org.
func TestRBAC(t *testing.T) {
	ts := newTestServer(t)

	ownerEmail := uniqueEmail(t, "owner")
	viewerEmail := uniqueEmail(t, "viewer")

	owner := signupAndLogin(t, ts, ownerEmail, "ownerpass1!")
	viewer := signupAndLogin(t, ts, viewerEmail, "viewerpass1!")

	// Owner creates an org.
	resp := owner.post("/api/v1/organizations", map[string]any{"name": "RBAC Test Org"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create org: %d", resp.StatusCode)
	}
	orgID := dataField(t, mustJSON(t, resp))["id"].(string)

	// Owner creates a team and project.
	teamID := createTeam(t, owner, orgID, "RBAC Team")
	projID := createProject(t, owner, teamID, "rbac-proj", "RBAC Project")

	// Invite viewer.
	var viewerID string
	_ = ts.pool.QueryRow(t.Context(), "SELECT id FROM users WHERE email = $1", viewerEmail).Scan(&viewerID)
	inviteResp := owner.post("/api/v1/organizations/"+orgID+"/invites",
		map[string]any{"email": viewerEmail, "role": "viewer"})
	if inviteResp.StatusCode != http.StatusCreated {
		t.Fatalf("invite: %d: %v", inviteResp.StatusCode, mustJSON(t, inviteResp))
	}
	token := dataField(t, mustJSON(t, inviteResp))["token"].(string)

	// Viewer accepts.
	acceptResp := viewer.post("/api/v1/invites/"+token+"/accept", nil)
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("accept invite: %d", acceptResp.StatusCode)
	}
	acceptResp.Body.Close()

	t.Run("viewer can read project", func(t *testing.T) {
		resp := viewer.get("/api/v1/projects/" + projID)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("viewer cannot delete project", func(t *testing.T) {
		resp := viewer.delete("/api/v1/projects/" + projID)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("viewer cannot delete org", func(t *testing.T) {
		resp := viewer.delete("/api/v1/organizations/" + orgID)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("viewer cannot create project", func(t *testing.T) {
		resp := viewer.post("/api/v1/teams/"+teamID+"/projects",
			map[string]any{"name": "Sneaky Project", "slug": "sneaky"})
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func createTeam(t *testing.T, c *client, orgID, name string) string {
	t.Helper()
	resp := c.post("/api/v1/organizations/"+orgID+"/teams", map[string]any{"name": name})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create team %q: %d: %v", name, resp.StatusCode, mustJSON(t, resp))
	}
	return dataField(t, mustJSON(t, resp))["id"].(string)
}

func createProject(t *testing.T, c *client, teamID, slug, name string) string {
	t.Helper()
	resp := c.post("/api/v1/teams/"+teamID+"/projects",
		map[string]any{"name": name, "slug": slug})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project %q: %d: %v", name, resp.StatusCode, mustJSON(t, resp))
	}
	return dataField(t, mustJSON(t, resp))["id"].(string)
}
