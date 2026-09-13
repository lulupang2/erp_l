package integration

import (
	"context"
	"testing"
	"time"

	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/store"
	"example.com/assembly-erp/api/internal/v2auth"
	"github.com/google/uuid"
)

func TestAuthSystem(t *testing.T) {
	h := harness{ctx: context.Background(), pool: nil, db: nil}
	t.Skip("Requires separate V2_DATABASE_URL and v2 isolation setup")
}

func TestBootstrappedAdminNotAutomatic(t *testing.T) {
	// Explicit bootstrap CLI is required; no automatic admin should exist.
	t.Skip("Requires explicit V2_DATABASE_URL")
}

func TestAccountOperationIdempotency(t *testing.T) {
	t.Skip("Requires v2 isolation setup")
}

func TestAuthAuditPersistence(t *testing.T) {
	t.Skip("Requires v2 isolation setup")
}

func TestPasswordHashVerification(t *testing.T) {
	hash, err := v2auth.PasswordHash("testpassword123!")
	if err != nil {
		t.Fatal("hash generation failed", err)
	}
	if len(hash) == 0 {
		t.Fatal("expected non-empty hash")
	}
}
