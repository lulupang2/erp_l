package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenV2 opens the factory schema connection. The portfolio uses one trusted
// server-side Neon connection because login and per-user authorization are
// explicitly outside this demo's scope.
func OpenV2(ctx context.Context, raw string) (*pgxpool.Pool, error) {
	return Open(ctx, raw)
}

// PrepareV2TestRole provisions only the explicitly isolated local test database.
// The random password is returned in memory, never printed or written to a file.
// Existing v1 rows and privileges are untouched; public grants that defeat
// isolation cause OpenV2 to fail rather than silently weakening protection.
func PrepareV2TestRole(ctx context.Context, raw string) (string, error) {
	if err := RequireLocalDatabase(raw, "erp_test", "55433"); err != nil {
		return "", err
	}
	pool, err := Open(ctx, raw)
	if err != nil {
		return "", err
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(98267103)"); err != nil {
		return "", err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='erp_v2_app')").Scan(&exists); err != nil {
		return "", err
	}
	if !exists {
		if _, err = tx.Exec(ctx, "CREATE ROLE erp_v2_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
			return "", err
		}
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return "", err
	}
	password := hex.EncodeToString(random[:])
	// Password is generated hex, not user input. PostgreSQL ALTER ROLE does not
	// accept a bind parameter in the password clause.
	if _, err = tx.Exec(ctx, "ALTER ROLE erp_v2_app PASSWORD '"+password+"'"); err != nil {
		return "", err
	}
	for _, statement := range []string{
		"GRANT CONNECT ON DATABASE erp_test TO erp_v2_app",
		"GRANT USAGE ON SCHEMA v2 TO erp_v2_app",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA v2 TO erp_v2_app",
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA v2 TO erp_v2_app",
		"REVOKE ALL ON v2.goose_db_version FROM erp_v2_app",
	} {
		if _, err = tx.Exec(ctx, statement); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	u, _ := url.Parse(raw)
	u.User = url.UserPassword("erp_v2_app", password)
	return u.String(), nil
}

// ProvisionV2Role grants an existing, operator-created nonprivileged role only
// v2 DML. Role creation/password handling remains an explicit operator action.
func ProvisionV2Role(ctx context.Context, raw, name string) error {
	if name == "" {
		return errors.New("an explicit v2 application role is required")
	}
	pool, err := Open(ctx, raw)
	if err != nil {
		return err
	}
	defer pool.Close()
	var safe bool
	if err = pool.QueryRow(ctx, `SELECT NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls) FROM pg_roles WHERE rolname=$1`, name).Scan(&safe); err != nil || !safe {
		return errors.New("v2 application role must already exist and be unprivileged")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	role := pgx.Identifier{name}.Sanitize()
	for _, statement := range []string{
		"GRANT USAGE ON SCHEMA v2 TO " + role,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA v2 TO " + role,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA v2 TO " + role,
		"REVOKE ALL ON v2.goose_db_version FROM " + role,
	} {
		if _, err = tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
