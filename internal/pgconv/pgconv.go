// Package pgconv converts between google/uuid, time.Time, and pgtype types.
package pgconv

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUID converts a uuid.UUID to pgtype.UUID.
func UUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// NullUUID converts a *uuid.UUID to pgtype.UUID, handling nil as NULL.
func NullUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil || *id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// FromUUID converts a pgtype.UUID back to uuid.UUID.
func FromUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}

// Timestamptz converts a time.Time to pgtype.Timestamptz.
func Timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// NullUUIDFromUUID wraps a uuid.UUID as a valid pgtype.UUID (non-nil path).
func NullUUIDFromUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// Addr parses an IP string into *netip.Addr for pgtype INET columns.
func Addr(ip string) *netip.Addr {
	if ip == "" {
		return nil
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return nil
	}
	return &a
}
