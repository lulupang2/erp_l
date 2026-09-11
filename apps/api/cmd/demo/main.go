package main

import (
	"context"
	"fmt"
	"os"

	"example.com/assembly-erp/api/internal/demo"
	"example.com/assembly-erp/api/internal/platform"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: demo seed | demo reset --confirm ERP_DEMO_RESET")
		os.Exit(1)
	}
	action, confirmation := os.Args[1], ""
	valid := action == "seed" && len(os.Args) == 2
	if action == "reset" && len(os.Args) == 4 && os.Args[2] == "--confirm" {
		confirmation = os.Args[3]
		valid = true
	}
	if !valid {
		fmt.Fprintln(os.Stderr, "Invalid demo command or confirmation arguments.")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
	defer cancel()
	if err := demo.Execute(ctx, os.Getenv("DATABASE_URL"), action, confirmation); err != nil {
		fmt.Fprintln(os.Stderr, "Demo operation refused or failed. Verify the dedicated local demo DB, schema, confirmation and existing seed data. Connection details were withheld.")
		os.Exit(1)
	}
	if action == "seed" {
		fmt.Println("Demo seed verified: 3 items, keyboard BOM and idempotent initial receipts. No production orders were created.")
	} else {
		fmt.Println("Explicit local demo reset completed; schema and other databases were preserved.")
	}
}
