package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/assembly-erp/api/internal/factory"
	"example.com/assembly-erp/api/internal/httpapi"
	"example.com/assembly-erp/api/internal/platform"
	"example.com/assembly-erp/api/internal/store"
)

func run() error {
	address, err := platform.ListenAddress(os.Getenv("API_ADDR"))
	if err != nil {
		return err
	}
	connect, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
	pool, err := platform.Open(connect, os.Getenv("DATABASE_URL"))
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	app := httpapi.New(store.New(pool))
	if raw := os.Getenv("V2_DATABASE_URL"); raw != "" {
		ctx, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
		v2pool, err := platform.OpenV2(ctx, raw)
		cancel()
		if err != nil {
			return err
		}
		defer v2pool.Close()
		service := factory.New(v2pool)
		factory.Register(app, service, factory.PublicAccess)
	}
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- app.Listen(address) }()
	select {
	case err := <-result:
		return err
	case <-shutdown.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return app.ShutdownWithContext(ctx)
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERP API failed to start or stopped unexpectedly. Check backend configuration, DB availability and the local port.")
		os.Exit(1)
	}
}
