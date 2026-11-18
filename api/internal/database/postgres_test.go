package database

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenRejectsInvalidConfiguration(t *testing.T) {
	pool, err := Open(context.Background(), "://invalid")
	if err == nil || pool != nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("expected invalid database configuration to be rejected")
	}
}

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("set DATABASE_TEST_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal("test PostgreSQL connection failed")
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "CREATE TEMP TABLE paycore_connection_check (amount BIGINT NOT NULL) ON COMMIT DROP"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO paycore_connection_check (amount) VALUES ($1)", int64(50000)); err != nil {
		t.Fatal(err)
	}
	var amount int64
	if err := tx.QueryRow(ctx, "SELECT amount FROM paycore_connection_check").Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 50000 {
		t.Fatalf("got %d, want 50000", amount)
	}
}
