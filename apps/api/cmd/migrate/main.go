package main

import (
	"context"
	"fmt"
	"os"

	"example.com/assembly-erp/api/internal/migrate"
	"example.com/assembly-erp/api/internal/platform"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 || (len(os.Args) == 3 && os.Args[2] != "--v2") {
		fmt.Fprintln(os.Stderr, "Usage: migrate up|status|down [--v2] (v2: up/status only; v1 down: isolated local test DB only)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
	defer cancel()
	var version int64
	var err error
	if len(os.Args) == 3 {
		version, err = migrate.ApplyV2(ctx, os.Getenv("V2_MIGRATION_DATABASE_URL"), os.Args[1])
	} else {
		version, err = migrate.Apply(ctx, os.Getenv("MIGRATION_DATABASE_URL"), os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Migration failed. Verify the explicit target, connectivity and migration SQL. Connection details were withheld.")
		os.Exit(1)
	}
	fmt.Printf("Migration %s verified; schema version=%d\n", os.Args[1], version)
}
