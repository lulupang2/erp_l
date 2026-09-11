package main

import (
	"context"
	"fmt"
	"os"

	"example.com/assembly-erp/api/internal/migrate"
	"example.com/assembly-erp/api/internal/platform"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: migrate up|status|down (down: isolated local test DB only)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
	defer cancel()
	version, err := migrate.Apply(ctx, os.Getenv("MIGRATION_DATABASE_URL"), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Migration failed. Verify the explicit target, connectivity and migration SQL. Connection details were withheld.")
		os.Exit(1)
	}
	fmt.Printf("Migration %s verified; schema version=%d\n", os.Args[1], version)
}
