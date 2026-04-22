package tenancy

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type contextKey int

const (
	keyOrgID contextKey = iota
	keyUserID
	keyAPIKeyID
	keyTx
)

// ErrNoTenantContext is returned when tenant context has not been set on the context.
var ErrNoTenantContext = errors.New("no tenant context in request")

// WithOrgID stores the resolved organization ID on the context.
func WithOrgID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, keyOrgID, id)
}

// OrgFromContext retrieves the organization ID from the context.
func OrgFromContext(ctx context.Context) (uuid.UUID, error) {
	v, ok := ctx.Value(keyOrgID).(uuid.UUID)
	if !ok || v == uuid.Nil {
		return uuid.Nil, ErrNoTenantContext
	}
	return v, nil
}

// WithUserID stores the authenticated user ID on the context.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, keyUserID, id)
}

// UserFromContext retrieves the user ID from the context (may be nil for API key auth).
func UserFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(keyUserID).(uuid.UUID)
	return v, ok && v != uuid.Nil
}

// WithAPIKeyID stores the authenticated API key ID on the context.
func WithAPIKeyID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, keyAPIKeyID, id)
}

// APIKeyFromContext retrieves the API key ID from the context (may be nil for session auth).
func APIKeyFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(keyAPIKeyID).(uuid.UUID)
	return v, ok && v != uuid.Nil
}

// WithTx stores the tenant-scoped pgx transaction on the context.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, keyTx, tx)
}

// TxFromContext retrieves the tenant-scoped transaction from the context.
func TxFromContext(ctx context.Context) (pgx.Tx, error) {
	v, ok := ctx.Value(keyTx).(pgx.Tx)
	if !ok || v == nil {
		return nil, errors.New("no transaction in context")
	}
	return v, nil
}
