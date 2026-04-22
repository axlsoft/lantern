package authz

import (
	"errors"

	"golang.org/x/crypto/argon2"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// argon2id parameters per ADR-011.
// Memory: 19 MiB, Iterations: 2, Parallelism: 1
const (
	argonMemory      = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonKeyLen      = 32
	argonSaltLen     = 16
)

// HashPassword returns an argon2id hash string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// CheckPassword returns nil if password matches the stored hash.
func CheckPassword(password, encoded string) error {
	parts := strings.Split(encoded, "$")
	// Format: $argon2id$v=N$m=M,t=T,p=P$salt$hash  → 6 segments after split on $
	if len(parts) != 6 {
		return errors.New("invalid hash format")
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return fmt.Errorf("parse hash params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return fmt.Errorf("decode hash: %w", err)
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), uint32(len(expected)))

	// Constant-time compare
	if len(got) != len(expected) {
		return errors.New("invalid password")
	}
	var diff byte
	for i := range got {
		diff |= got[i] ^ expected[i]
	}
	if diff != 0 {
		return errors.New("invalid password")
	}
	return nil
}
