// dbprobe makes read-only checks against the explicitly selected Neon profile.
// It never migrates, seeds, deletes data, prints credentials, or runs periodically.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"example.com/assembly-erp/api/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var stage = "profile"

func run() error {
	raw := os.Getenv("DATABASE_URL")
	u, err := platform.ValidateURL(raw)
	if err != nil || u.Hostname() != os.Getenv("NEON_HOST") || u.Path != "/"+os.Getenv("NEON_DATABASE") ||
		!strings.HasSuffix(u.Hostname(), ".neon.tech") || strings.Contains(u.Hostname(), "-pooler.") || u.Query().Get("sslmode") != "verify-full" {
		return errors.New("explicit direct Neon target required")
	}
	stage = "pool-config"
	config, err := platform.PoolConfig(raw)
	if err != nil {
		return err
	}
	mode := "probe"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode != "probe" && mode != "negative-tls" && mode != "negative-auth" {
		return errors.New("unknown probe")
	}
	if mode == "negative-tls" {
		config.ConnConfig.TLSConfig.ServerName = "invalid-certificate.example.invalid"
	}
	if mode == "negative-auth" {
		config.ConnConfig.Password += "-invalid-probe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
	defer cancel()
	start := time.Now()
	stage = "connect"
	conn, err := pgx.ConnectConfig(ctx, config.ConnConfig)
	if conn != nil {
		defer conn.Close(context.Background())
	}
	if mode != "probe" {
		var hostname x509.HostnameError
		var postgres *pgconn.PgError
		ok := mode == "negative-tls" && errors.As(err, &hostname) || mode == "negative-auth" && errors.As(err, &postgres) && postgres.Code == "28P01"
		if !ok {
			return errors.New("expected authentication/certificate rejection was not verified")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"check": mode, "expected_rejection": true})
	}
	if err != nil {
		return err
	}
	stage = "tls-state"
	tlsConn, ok := conn.PgConn().Conn().(*tls.Conn)
	if !ok {
		return errors.New("connection is not TLS")
	}
	state := tlsConn.ConnectionState()
	if !state.HandshakeComplete || len(state.VerifiedChains) == 0 || config.ConnConfig.TLSConfig.InsecureSkipVerify {
		return errors.New("certificate chain not verified")
	}
	var version int
	var database string
	var ssl bool
	var protocol string
	var started time.Time
	stage = "server-details"
	err = conn.QueryRow(ctx, `SELECT current_setting('server_version_num')::int, current_database(),
		ssl, COALESCE(version,''), pg_postmaster_start_time() FROM pg_stat_ssl WHERE pid=pg_backend_pid()`).Scan(&version, &database, &ssl, &protocol, &started)
	if err != nil {
		return err
	}
	if version/10000 != 17 || database != os.Getenv("NEON_DATABASE") {
		return errors.New("PostgreSQL version or target verification failed")
	}
	var schemaVersion int64
	stage = "migration-version"
	if err = conn.QueryRow(ctx, `SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&schemaVersion); err != nil {
		return err
	}
	var mismatches int64
	stage = "ledger"
	if err = conn.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM inventory_balances b WHERE b.quantity <> COALESCE((SELECT sum(delta) FROM stock_movements m WHERE m.item_id=b.item_id),0)) +
		(SELECT count(*) FROM production_orders o WHERE o.good_quantity <> COALESCE((SELECT sum(good_quantity) FROM production_results r WHERE r.order_id=o.id),0)
		OR o.defective_quantity <> COALESCE((SELECT sum(defective_quantity) FROM production_results r WHERE r.order_id=o.id),0))`).Scan(&mismatches); err != nil {
		return err
	}
	if mismatches != 0 {
		return errors.New("ledger mismatch")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		// pg_stat_ssl describes the backend hop, not the client TLS session at a proxy.
		// Certificate verification comes from the actual client handshake above.
		"check": "probe", "postgres_major": version / 10000, "tls": tls.VersionName(state.Version), "certificate_verified": true,
		"postgres_backend_tls": ssl,
		"direct_connection":    true, "migration_version": schemaVersion, "ledger_mismatches": mismatches,
		"probe_ms": time.Since(start).Milliseconds(), "postgres_started_at": started.UTC(), "checked_at": time.Now().UTC(),
	})
}

func main() {
	if err := run(); err != nil {
		var postgres *pgconn.PgError
		state := ""
		if errors.As(err, &postgres) {
			state = postgres.Code
		}
		fmt.Fprintf(os.Stderr, "Neon database probe failed at %s (SQLSTATE=%s, type=%T). Connection details were withheld.\n", stage, state, err)
		os.Exit(1)
	}
}
