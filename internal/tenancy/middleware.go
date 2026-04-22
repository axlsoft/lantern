package tenancy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/httperr"
)

// TenantMiddleware opens a transaction, sets lantern.current_organization_id,
// and commits (or rolls back) when the handler returns. It requires the org ID
// to already be present on the context (placed there by session or API key auth).
func TenantMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID, err := OrgFromContext(r.Context())
			if err != nil {
				httperr.Unauthorized(w, "authentication required")
				return
			}

			ctx, tx, err := beginTenantTx(r.Context(), pool, orgID)
			if err != nil {
				httperr.Internal(w, "could not open transaction")
				return
			}

			ww := &responseWriterCapture{ResponseWriter: w}
			next.ServeHTTP(ww, r.WithContext(ctx))

			if ww.Status() >= 400 {
				_ = tx.Rollback(context.Background())
				return
			}
			if err := tx.Commit(context.Background()); err != nil {
				_ = tx.Rollback(context.Background())
			}
		})
	}
}

func beginTenantTx(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID) (context.Context, pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ctx, nil, fmt.Errorf("begin tx: %w", err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lantern.current_organization_id = '%s'", orgID.String())); err != nil {
		_ = tx.Rollback(ctx)
		return ctx, nil, fmt.Errorf("set tenant: %w", err)
	}

	ctx = WithTx(ctx, tx)
	return ctx, tx, nil
}

// responseWriterCapture captures the HTTP status code written by a handler.
type responseWriterCapture struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriterCapture) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterCapture) Status() int {
	if rw.status == 0 {
		return http.StatusOK
	}
	return rw.status
}
