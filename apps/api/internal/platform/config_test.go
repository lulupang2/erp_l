package platform

import (
	"testing"
	"time"
)

func TestPoolConfiguration(t *testing.T) {
	cfg, err := PoolConfig("postgres://erp@127.0.0.1:55433/erp_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 5 || cfg.MinConns != 0 || cfg.ConnConfig.ConnectTimeout != 15*time.Second {
		t.Fatal("pool defaults differ from SSOT")
	}
	remote, err := PoolConfig("postgres://erp@example.neon.tech/erp?sslmode=verify-full")
	if err != nil {
		t.Fatal(err)
	}
	if remote.ConnConfig.TLSConfig == nil || remote.ConnConfig.TLSConfig.InsecureSkipVerify || remote.ConnConfig.TLSConfig.ServerName != "example.neon.tech" {
		t.Fatal("remote TLS verification disabled")
	}
	for _, fallback := range remote.ConnConfig.Fallbacks {
		if fallback.TLSConfig == nil {
			t.Fatal("plaintext remote fallback configured")
		}
	}
}

func TestUnsafeConfigurationRejected(t *testing.T) {
	for _, value := range []string{"", "not a URL", "https://localhost/erp", "postgres:///erp", "postgres://remote.test/erp",
		"postgres://remote.test/erp?sslmode=require", "postgres://remote.test/erp?sslmode=disable",
		"postgres://remote.test/erp?sslmode=verify-full&sslmode=disable", "postgres://remote.test/erp?sslmode=verify-full&bad=%zz",
		"postgres://localhost:55433/erp_test?host=remote.test", "postgres://localhost:55433/erp_test?options=-csearch_path=public"} {
		if _, err := PoolConfig(value); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	for _, address := range []string{"0.0.0.0:8080", ":8080", "[::]:8080", "remote.test:8080", "127.0.0.1:0", "127.0.0.1:65536"} {
		if _, err := ListenAddress(address); err == nil {
			t.Fatal("nonlocal or invalid bind accepted")
		}
	}
	for _, address := range []string{"", "127.0.0.1:8080", "localhost:18080", "[::1]:18080"} {
		if _, err := ListenAddress(address); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequestHostAllowed(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "ERP.JISUNG.LOL", "erp.jisung.lol."} {
		if !RequestHostAllowed(host, "erp.jisung.lol") {
			t.Fatalf("expected host %q to be allowed", host)
		}
	}
	for _, host := range []string{"example.com", "erp.jisung.lol.evil.example", "10.0.0.2"} {
		if RequestHostAllowed(host, "erp.jisung.lol") {
			t.Fatalf("unexpected host %q allowed", host)
		}
	}
	for _, configured := range []string{"", "https://erp.jisung.lol", "erp.jisung.lol:443", "127.0.0.1"} {
		if RequestHostAllowed("erp.jisung.lol", configured) {
			t.Fatalf("invalid public host %q accepted", configured)
		}
	}
}

func TestDestructiveTargetGuard(t *testing.T) {
	if err := RequireLocalDatabase("postgres://erp@127.0.0.1:55432/erp_demo?sslmode=disable", "erp_demo", "55432"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"postgres://localhost:55433/erp_test", "postgres://localhost:5432/erp_demo", "postgres://remote.test:55432/erp_demo?sslmode=verify-full",
		"postgres://localhost:55432/erp_demo?dbname=other", "postgres://localhost:55432/other"} {
		if RequireLocalDatabase(raw, "erp_demo", "55432") == nil {
			t.Fatal("unsafe demo reset target accepted")
		}
	}
}
