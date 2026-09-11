package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
