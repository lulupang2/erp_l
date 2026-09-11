# Backend implementation rules

Read the current files and docs/PROGRESS.md before changing them. Preserve existing
work and coordinate file ownership when several workers are active.

SQL in db/queries and Goose migrations are authoritative. Generate internal/db with
sqlc v1.31.1; never hand-edit generated code. Services use standard context.Context,
separate DTOs and one transaction-bound sqlc Queries for each write operation.
Tests may only reset the explicitly isolated loopback erp_test database.
