package migrate

import (
	"context"
	"errors"

	"example.com/assembly-erp/api/db/migrations"
	"example.com/assembly-erp/api/internal/platform"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type quietLogger struct{}

func (quietLogger) Printf(string, ...any) {}
func (quietLogger) Fatalf(string, ...any) { panic("migration logger reported a fatal error") }

// Apply is called explicitly by the migration CLI/tests, never server startup.
func Apply(ctx context.Context, rawURL, action string) (int64, error) {
	if action != "up" && action != "down" && action != "status" {
		return 0, errors.New("unsupported migration command")
	}
	if action == "down" {
		if err := platform.RequireLocalDatabase(rawURL, "erp_test", "55433"); err != nil {
			return 0, err
		}
	}
	config, err := platform.PoolConfig(rawURL)
	if err != nil {
		return 0, err
	}
	database := stdlib.OpenDB(*config.ConnConfig)
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(0)
	if err = database.PingContext(ctx); err != nil {
		return 0, err
	}
	goose.SetBaseFS(migrations.Files)
	goose.SetLogger(quietLogger{})
	if err = goose.SetDialect("postgres"); err != nil {
		return 0, err
	}
	switch action {
	case "up":
		err = goose.UpContext(ctx, database, ".")
	case "down":
		err = goose.DownContext(ctx, database, ".")
	}
	if err != nil {
		return 0, err
	}
	return goose.GetDBVersionContext(ctx, database)
}
