# Lantern Wire Protocol Specification

**Schema version:** 1  
**Status:** Active

This document defines the contracts between Lantern SDKs, the Playwright plugin, and the collector. All parties must agree on these formats before they can interoperate.

---

## 1. Coverage Batch Wire Format

### Endpoint

```
POST /v1/coverage
Authorization: Bearer <api-key>
Content-Type: application/x-protobuf   (preferred)
              application/json          (debug only)
```

### Protobuf message: `CoverageBatch`

Defined in `proto/lantern/v1/coverage.proto`. Key fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `batch_id` | string | yes | Client-generated UUID; used for idempotency. Re-submitting the same `batch_id` is a no-op. |
| `resource` | Resource | yes | Shared context for all events in the batch. |
| `events` | Coverage[] | yes | One entry per file with coverage hits. |

### Resource fields

| Field | Notes |
|---|---|
| `schema_version` | Must be `"1"`. Batches with other values are rejected with HTTP 400. |
| `project_id` | UUID string. Must match the project scoped to the API key. |
| `run_id` | UUID string. Must reference an existing run created via `POST /v1/runs`. |
| `commit_sha` | Git commit SHA. Set from `LANTERN_COMMIT_SHA`, `GITHUB_SHA`, or `GIT_COMMIT_SHA` env vars; defaults to `"unknown"`. |
| `sdk_name` | e.g. `"lantern-dotnet"` |
| `sdk_version` | semver string |

### Coverage event fields

| Field | Notes |
|---|---|
| `test_id` | UUID of the test row created via `POST /v1/runs/:run_id/tests`. Optional; populated in SERIALIZED mode. |
| `file_path` | Repo-relative path, forward-slash separated. |
| `line_start` / `line_end` | Inclusive line range with hits. |
| `hit_count` | Max observed hit count across the coverage window. |
| `attribution_mode` | `SERIALIZED` in MVP. |

---

## 2. Traceparent + Baggage Injection Convention

The Playwright plugin injects an HTTP request that the .NET SDK's middleware uses to establish a test scope. Two W3C headers carry the Lantern context.

### 2.1 `traceparent` header

A **valid W3C traceparent** header must be present. The middleware rejects (no Lantern scope established) any request without one.

```
traceparent: 00-<32-hex-trace-id>-<16-hex-span-id>-<flags>
```

The trace-id can be any valid 32-char lowercase hex string. The Playwright plugin typically uses a random UUID (stripped of dashes) as the trace-id so OTel spans from the request are correlated in any existing tracing backend.

Example:
```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
```

### 2.2 `baggage` header (W3C Baggage)

The test identifier and human-readable name are carried in the W3C `baggage` header:

```
baggage: lantern.test_id=<testId>,lantern.test_name=<url-encoded-name>
```

| Baggage key | Required | Notes |
|---|---|---|
| `lantern.test_id` | yes | Opaque string, max 128 chars. Must be URL-safe or percent-encoded. Typically a UUID or truncated UUID. |
| `lantern.test_name` | no | Human-readable test name, percent-encoded. |

**Both headers must be present.** A request with only `traceparent` and no `lantern.test_id` baggage is treated as a normal (unscoped) request.

### 2.3 Activation sequence (middleware path)

```
Request arrives
  │
  ├─ No traceparent?          → pass through, no scope
  ├─ Invalid traceparent?     → pass through, no scope
  ├─ No lantern.test_id baggage? → pass through, no scope
  └─ Valid traceparent + lantern.test_id baggage
         → BeginTestScope(testId, testName)
         → next(context)
         → EndTestScope(testId)
         → coverage event enqueued
```

### 2.4 Control plane path (alternative activation)

For cases where the Playwright plugin cannot inject headers into individual requests (e.g. non-HTTP workers), the control plane endpoints provide explicit scope management:

```
POST /_lantern/test/start?test_id=<id>&test_name=<name>
POST /_lantern/test/stop?test_id=<id>
GET  /_lantern/health
```

The `test_id` may alternatively be passed via `X-Lantern-Test-Id` header.

---

## 3. Run Lifecycle Protocol

Before submitting coverage events, the caller must create a run:

```
POST /v1/runs
{
  "project_id": "<uuid>",
  "commit_sha": "<sha>",
  "branch": "main",          // optional
  "attribution_mode": "serialized"
}
→ { "data": { "id": "<run-uuid>", ... } }
```

Register tests within the run:

```
POST /v1/runs/<run-id>/tests
{
  "tests": [
    { "test_external_id": "<id>", "name": "<name>", "suite": "<suite>", "file_path": "<path>" }
  ]
}
→ { "data": [{ "id": "<test-uuid>", "test_external_id": "<id>", ... }] }
```

Submit coverage (see §1), then close the run:

```
PATCH /v1/runs/<run-id>
{ "status": "completed", "total_tests": 5, "passed_tests": 5, "failed_tests": 0, "skipped_tests": 0 }
```

---

## 4. Schema Version Compatibility

| Collector version | Accepted `schema_version` values |
|---|---|
| 1.x | `"1"` |

SDKs must set `Resource.schema_version = "1"`. Batches with unknown schema versions are rejected with HTTP 400 and a structured error body referencing this document.

---

## 5. API Key Authentication

All ingestion endpoints require:

```
Authorization: Bearer lntn_<40-char-base62-secret>
```

The API key is scoped to a single project. The `project_id` in the request payload must match the project the key was issued for; mismatches return HTTP 403.
