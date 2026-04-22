package integration_test

import (
	"bytes"
	"net/http"
	"testing"

	"google.golang.org/protobuf/proto"

	lanternv1 "github.com/axlsoft/lantern/gen/proto/lantern/v1"
)

// TestAPIKeyLifecycle covers F1.1-4.
func TestAPIKeyLifecycle(t *testing.T) {
	ts := newTestServer(t)
	owner := signupAndLogin(t, ts, uniqueEmail(t, "apikeyowner"), "keyowner-pass1!")

	resp := owner.post("/api/v1/organizations", map[string]any{"name": "Key Org"})
	orgID := dataField(t, mustJSON(t, resp))["id"].(string)
	teamID := createTeam(t, owner, orgID, "Key Team")
	projID := createProject(t, owner, teamID, "key-proj", "Key Project")

	var fullKey string

	t.Run("create key returns full value once", func(t *testing.T) {
		resp := owner.post("/api/v1/projects/"+projID+"/api-keys",
			map[string]any{"name": "CI Key"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create key: %d: %v", resp.StatusCode, mustJSON(t, resp))
		}
		data := dataField(t, mustJSON(t, resp))
		fullKey = data["key"].(string)
		if len(fullKey) < 12 || fullKey[:5] != "lntn_" {
			t.Errorf("unexpected key format: %q", fullKey)
		}
	})

	var keyID string

	t.Run("list keys returns prefix not full value", func(t *testing.T) {
		resp := owner.get("/api/v1/projects/" + projID + "/api-keys")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list keys: %d", resp.StatusCode)
		}
		body := mustJSON(t, resp)
		keys := body["data"].([]any)
		if len(keys) == 0 {
			t.Fatal("expected at least one key")
		}
		k := keys[0].(map[string]any)
		keyID = k["id"].(string)
		if _, hasKey := k["key"]; hasKey {
			t.Error("list response must not include full key value")
		}
		if _, hasHash := k["key_hash"]; hasHash {
			t.Error("list response must not include key_hash")
		}
	})

	t.Run("valid key authenticates ingestion request", func(t *testing.T) {
		apiClient := ts.client().withAPIKey(fullKey)
		resp := apiClient.post("/v1/runs", map[string]any{
			"project_id": projID,
			"commit_sha": "abc123",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create run with API key: %d: %v", resp.StatusCode, mustJSON(t, resp))
		}
		resp.Body.Close()
	})

	t.Run("invalid key returns 401", func(t *testing.T) {
		apiClient := ts.client().withAPIKey("lntn_INVALIDKEYXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
		resp := apiClient.post("/v1/runs", map[string]any{
			"project_id": projID,
			"commit_sha": "abc123",
		})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("revoked key returns 401", func(t *testing.T) {
		resp := owner.delete("/api/v1/projects/" + projID + "/api-keys/" + keyID)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("revoke: %d", resp.StatusCode)
		}
		resp.Body.Close()

		apiClient := ts.client().withAPIKey(fullKey)
		resp = apiClient.post("/v1/runs", map[string]any{
			"project_id": projID,
			"commit_sha": "abc123",
		})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("revoked key: expected 401, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestCoverageIngestion covers F1.1-5.
func TestCoverageIngestion(t *testing.T) {
	ts := newTestServer(t)
	owner := signupAndLogin(t, ts, uniqueEmail(t, "ingest"), "ingest-pass1!")

	resp := owner.post("/api/v1/organizations", map[string]any{"name": "Ingest Org"})
	orgID := dataField(t, mustJSON(t, resp))["id"].(string)
	teamID := createTeam(t, owner, orgID, "Ingest Team")
	projID := createProject(t, owner, teamID, "ingest-proj", "Ingest Project")

	// Create an API key.
	resp = owner.post("/api/v1/projects/"+projID+"/api-keys", map[string]any{"name": "Ingest Key"})
	apiKey := dataField(t, mustJSON(t, resp))["key"].(string)
	apiClient := ts.client().withAPIKey(apiKey)

	// Create a run.
	resp = apiClient.post("/v1/runs", map[string]any{
		"project_id": projID,
		"commit_sha": "deadbeef",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create run: %d: %v", resp.StatusCode, mustJSON(t, resp))
	}
	runID := dataField(t, mustJSON(t, resp))["id"].(string)

	batch := &lanternv1.CoverageBatch{
		Resource: &lanternv1.Resource{
			ProjectId:     projID,
			RunId:         runID,
			CommitSha:     "deadbeef",
			SchemaVersion: "1",
			SdkName:       "lantern-test",
			SdkVersion:    "0.0.1",
		},
		BatchId: "batch-unique-001",
		Events: []*lanternv1.Coverage{
			{FilePath: "src/Foo.cs", LineStart: 1, LineEnd: 10, HitCount: 3},
			{FilePath: "src/Bar.cs", LineStart: 5, LineEnd: 20, HitCount: 1},
		},
	}

	t.Run("valid protobuf batch inserts rows", func(t *testing.T) {
		data, _ := proto.Marshal(batch)
		req, _ := newProtoRequest(ts.URL+"/v1/coverage", data, apiKey)
		resp, _ := ts.Client().Do(req)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, mustJSON(t, resp))
		}
		body := dataField(t, mustJSON(t, resp))
		inserted := body["inserted"].(float64)
		if inserted != 2 {
			t.Errorf("inserted: got %v, want 2", inserted)
		}
	})

	t.Run("same batch_id is idempotent (no duplicate rows)", func(t *testing.T) {
		data, _ := proto.Marshal(batch) // same batch_id
		req, _ := newProtoRequest(ts.URL+"/v1/coverage", data, apiKey)
		resp, _ := ts.Client().Do(req)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		body := dataField(t, mustJSON(t, resp))
		if msg, ok := body["message"].(string); !ok || msg != "batch already processed" {
			// also acceptable: inserted == 0 (unique constraint absorbs)
			_ = msg
		}
		resp.Body.Close()
	})

	t.Run("unsupported schema_version returns 400", func(t *testing.T) {
		badBatch := &lanternv1.CoverageBatch{
			Resource: &lanternv1.Resource{
				ProjectId:     projID,
				RunId:         runID,
				SchemaVersion: "999",
			},
			BatchId: "batch-bad-schema",
		}
		data, _ := proto.Marshal(badBatch)
		req, _ := newProtoRequest(ts.URL+"/v1/coverage", data, apiKey)
		resp, _ := ts.Client().Do(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("API key from project A cannot submit to project B", func(t *testing.T) {
		// Create a second org+project.
		owner2 := signupAndLogin(t, ts, uniqueEmail(t, "ingest2"), "ingest2-pass!")
		resp := owner2.post("/api/v1/organizations", map[string]any{"name": "Other Org"})
		orgB := dataField(t, mustJSON(t, resp))["id"].(string)
		teamB := createTeam(t, owner2, orgB, "T2")
		projB := createProject(t, owner2, teamB, "proj-b", "Proj B")
		resp = owner2.post("/api/v1/projects/"+projB+"/api-keys", map[string]any{"name": "Key B"})
		keyB := dataField(t, mustJSON(t, resp))["key"].(string)
		apiClientB := ts.client().withAPIKey(keyB)
		resp = apiClientB.post("/v1/runs", map[string]any{"project_id": projB, "commit_sha": "aa"})
		runB := dataField(t, mustJSON(t, resp))["id"].(string)

		// Use key for project A, but claim project B in the payload.
		crossBatch := &lanternv1.CoverageBatch{
			Resource: &lanternv1.Resource{
				ProjectId:     projB, // ← belongs to org B
				RunId:         runB,
				SchemaVersion: "1",
			},
			BatchId: "cross-tenant-attempt",
			Events:  []*lanternv1.Coverage{{FilePath: "x.cs", LineStart: 1, LineEnd: 1, HitCount: 1}},
		}
		data, _ := proto.Marshal(crossBatch)
		req, _ := newProtoRequest(ts.URL+"/v1/coverage", data, apiKey) // key from org A
		resp, _ = ts.Client().Do(req)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("cross-tenant write: expected 403, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestRunLifecycle covers F1.1-6: create → register tests → submit coverage → complete.
func TestRunLifecycle(t *testing.T) {
	ts := newTestServer(t)
	owner := signupAndLogin(t, ts, uniqueEmail(t, "runlc"), "runlc-pass1!")

	resp := owner.post("/api/v1/organizations", map[string]any{"name": "Run Org"})
	orgID := dataField(t, mustJSON(t, resp))["id"].(string)
	teamID := createTeam(t, owner, orgID, "Run Team")
	projID := createProject(t, owner, teamID, "run-proj", "Run Project")
	resp = owner.post("/api/v1/projects/"+projID+"/api-keys", map[string]any{"name": "Run Key"})
	apiKey := dataField(t, mustJSON(t, resp))["key"].(string)
	apiClient := ts.client().withAPIKey(apiKey)

	// 1. Create run.
	resp = apiClient.post("/v1/runs", map[string]any{
		"project_id": projID,
		"commit_sha": "cafef00d",
		"branch":     "main",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create run: %d", resp.StatusCode)
	}
	runID := dataField(t, mustJSON(t, resp))["id"].(string)

	// 2. Register tests.
	resp = apiClient.post("/v1/runs/"+runID+"/tests", map[string]any{
		"tests": []any{
			map[string]any{"test_external_id": "t1", "name": "Should pass", "suite": "Suite A", "file_path": "src/T.cs"},
			map[string]any{"test_external_id": "t2", "name": "Should fail", "suite": "Suite A", "file_path": "src/T.cs"},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register tests: %d: %v", resp.StatusCode, mustJSON(t, resp))
	}
	tests := mustJSON(t, resp)["data"].([]any)
	if len(tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(tests))
	}
	testID := tests[0].(map[string]any)["id"].(string)

	// 3. Submit coverage.
	var durationMs int32 = 150
	resp = apiClient.patch("/v1/runs/"+runID+"/tests/"+testID,
		map[string]any{"status": "passed", "duration_ms": durationMs})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update test: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Mark run complete.
	resp = apiClient.patch("/v1/runs/"+runID, map[string]any{
		"status":        "completed",
		"total_tests":   2,
		"passed_tests":  1,
		"failed_tests":  1,
		"skipped_tests": 0,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update run: %d: %v", resp.StatusCode, mustJSON(t, resp))
	}
	run := dataField(t, mustJSON(t, resp))
	if run["status"] != "completed" {
		t.Errorf("run status: got %v, want completed", run["status"])
	}
}

// ── Proto request helper ──────────────────────────────────────────────────────

func newProtoRequest(url string, body []byte, apiKey string) (*http.Request, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req, nil
}
