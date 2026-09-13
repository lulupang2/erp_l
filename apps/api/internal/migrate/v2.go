package migrate

import (
	"context"
	"errors"

	"example.com/assembly-erp/api/db/v2migrations"
	"example.com/assembly-erp/api/internal/platform"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// ApplyV2 is explicit and never called by server startup. Its migration history
// lives in v2; no v1 migration or data is reinterpreted. Reversal is a business
// operation, not a migration down.
func ApplyV2(ctx context.Context, rawURL, action string) (int64, error) {
	if action != "up" && action != "status" {
		return 0, errors.New("v2 migrations support only up and status")
	}
	config, err := platform.PoolConfig(rawURL)
	if err != nil {
		return 0, err
	}
	database := stdlib.OpenDB(*config.ConnConfig)
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(0)
	if action == "up" {
		if _, err = database.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS v2"); err != nil {
			return 0, err
		}
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, database, v2migrations.Files, goose.WithTableName("v2.goose_db_version"))
	if err != nil {
		return 0, err
	}
	if action == "up" {
		if _, err = provider.Up(ctx); err != nil {
			return 0, err
		}
	}
	return provider.GetDBVersion(ctx)
}
