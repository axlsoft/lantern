package audit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/google/uuid"

	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

// Log writes an audit entry for the current request. Best-effort; errors are dropped.
func Log(ctx context.Context, q *generated.Queries, r *http.Request, action, resourceType, resourceID string, metadata map[string]any) {
	orgID, err := tenancy.OrgFromContext(ctx)
	if err != nil {
		return
	}

	raw, _ := json.Marshal(metadata)

	var actorUserID *uuid.UUID
	if uid, ok := tenancy.UserFromContext(ctx); ok {
		u := uid
		actorUserID = &u
	}

	var actorAPIKeyID *uuid.UUID
	if kid, ok := tenancy.APIKeyFromContext(ctx); ok {
		k := kid
		actorAPIKeyID = &k
	}

	ipStr := ""
	if r != nil {
		ipStr = extractIP(r)
	}

	userAgent := ""
	if r != nil {
		userAgent = r.UserAgent()
	}

	_ = q.CreateAuditEntry(ctx, generated.CreateAuditEntryParams{
		OrganizationID: pgconv.UUID(orgID),
		ActorUserID:    pgconv.NullUUID(actorUserID),
		ActorApiKeyID:  pgconv.NullUUID(actorAPIKeyID),
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		Metadata:       raw,
		IpAddress:      pgconv.Addr(ipStr),
		UserAgent:      &userAgent,
	})
}

func extractIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
