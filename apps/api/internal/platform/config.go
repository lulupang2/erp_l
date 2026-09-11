package platform

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const OperationTimeout = 20 * time.Second

func Loopback(host string) bool {
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

// ValidateURL rejects options that can silently override the visible host/database.
func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" || u.Fragment != "" {
		return nil, errors.New("a valid PostgreSQL connection URL is required")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, errors.New("connection options are invalid")
	}
	for _, values := range query {
		if len(values) != 1 {
			return nil, errors.New("duplicate connection options are not allowed")
		}
	}
	for _, key := range []string{"host", "hostaddr", "port", "database", "dbname", "service", "options"} {
		if u.Query().Has(key) {
			return nil, errors.New("connection target override options are not allowed")
		}
	}
	if !Loopback(u.Hostname()) && u.Query().Get("sslmode") != "verify-full" {
		return nil, errors.New("nonlocal PostgreSQL requires sslmode=verify-full")
	}
	return u, nil
}

func PoolConfig(raw string) (*pgxpool.Config, error) {
	if _, err := ValidateURL(raw); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return nil, errors.New("PostgreSQL connection configuration is invalid")
	}
	cfg.MaxConns, cfg.MinConns = 5, 0
	cfg.ConnConfig.ConnectTimeout = 15 * time.Second
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "20000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "15000"
	return cfg, nil
}

func Open(ctx context.Context, raw string) (*pgxpool.Pool, error) {
	cfg, err := PoolConfig(raw)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("PostgreSQL pool could not be created")
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("PostgreSQL connection verification failed")
	}
	return pool, nil
}

func ListenAddress(raw string) (string, error) {
	if raw == "" {
		return "127.0.0.1:8080", nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || !Loopback(host) {
		return "", errors.New("API_ADDR must bind to a loopback address")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("API_ADDR port is invalid")
	}
	return raw, nil
}

func RequireLocalDatabase(raw, name, port string) error {
	u, err := ValidateURL(raw)
	if err != nil || !Loopback(u.Hostname()) || u.Path != "/"+name || u.Port() != port {
		return errors.New("operation refused: an explicitly designated local ERP database is required")
	}
	return nil
}
