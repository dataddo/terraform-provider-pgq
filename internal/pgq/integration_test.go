//go:build integration

package pgq

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	connStr := fmt.Sprintf(
		"host=%s port=%s database=%s user=%s password=%s sslmode=disable",
		getEnv("PGHOST", "localhost"),
		getEnv("PGPORT", "5432"),
		getEnv("PGDATABASE", "postgres"),
		getEnv("PGUSER", "postgres"),
		getEnv("PGPASSWORD", ""),
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}

	return pool
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestManagerSimpleQueue(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	mgr := NewManager(pool)
	ctx := context.Background()

	schema := SchemaName("public")
	name := QueueName(fmt.Sprintf("test_simple_%d", os.Getpid()))

	defer mgr.Drop(ctx, schema, name)

	if err := mgr.CreateSimple(ctx, schema, name); err != nil {
		t.Fatalf("CreateSimple() error = %v", err)
	}

	exists, err := mgr.Exists(ctx, schema, name)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("queue should exist after creation")
	}

	q, err := mgr.Get(ctx, schema, name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if q.Partitioned {
		t.Error("simple queue should not be partitioned")
	}

	if err := mgr.CreateSimple(ctx, schema, name); err == nil {
		t.Error("creating duplicate queue should fail")
	}
}

func TestManagerPartitionedQueue(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	mgr := NewManager(pool)
	ctx := context.Background()

	schema := SchemaName("public")
	name := QueueName(fmt.Sprintf("test_part_%d", os.Getpid()))

	defer mgr.Drop(ctx, schema, name)
	defer mgr.RemovePartmanConfig(ctx, schema, name)

	cfg := &PartitionConfig{
		Interval:           "1 day",
		Premake:            3,
		Retention:          "7 days",
		DatetimeString:     "YYYYMMDD",
		OptimizeConstraint: 10,
		DefaultPartition:   true,
	}

	if err := mgr.CreatePartitioned(ctx, schema, name, cfg); err != nil {
		t.Fatalf("CreatePartitioned() error = %v", err)
	}

	q, err := mgr.Get(ctx, schema, name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !q.Partitioned {
		t.Error("partitioned queue should be marked as partitioned")
	}

	gotCfg, err := mgr.GetPartitionConfig(ctx, schema, name)
	if err != nil {
		t.Fatalf("GetPartitionConfig() error = %v", err)
	}

	if gotCfg.Interval != cfg.Interval {
		t.Errorf("interval = %q, want %q", gotCfg.Interval, cfg.Interval)
	}
	if gotCfg.Premake != cfg.Premake {
		t.Errorf("premake = %d, want %d", gotCfg.Premake, cfg.Premake)
	}

	newCfg := &PartitionConfig{
		Interval:           "1 day",
		Premake:            5,
		Retention:          "14 days",
		DatetimeString:     "YYYYMMDD",
		OptimizeConstraint: 20,
		DefaultPartition:   true,
	}

	if err := mgr.UpdatePartitionConfig(ctx, schema, name, newCfg); err != nil {
		t.Fatalf("UpdatePartitionConfig() error = %v", err)
	}

	updatedCfg, err := mgr.GetPartitionConfig(ctx, schema, name)
	if err != nil {
		t.Fatalf("GetPartitionConfig() after update error = %v", err)
	}

	if updatedCfg.Premake != newCfg.Premake {
		t.Errorf("after update: premake = %d, want %d", updatedCfg.Premake, newCfg.Premake)
	}
	if updatedCfg.Retention != newCfg.Retention {
		t.Errorf("after update: retention = %q, want %q", updatedCfg.Retention, newCfg.Retention)
	}
}

func TestManagerDrop(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	mgr := NewManager(pool)
	ctx := context.Background()

	schema := SchemaName("public")
	name := QueueName(fmt.Sprintf("test_drop_%d", os.Getpid()))

	if err := mgr.CreateSimple(ctx, schema, name); err != nil {
		t.Fatalf("CreateSimple() error = %v", err)
	}

	if err := mgr.Drop(ctx, schema, name); err != nil {
		t.Fatalf("Drop() error = %v", err)
	}

	exists, err := mgr.Exists(ctx, schema, name)
	if err != nil {
		t.Fatalf("Exists() after drop error = %v", err)
	}
	if exists {
		t.Error("queue should not exist after drop")
	}

	if err := mgr.Drop(ctx, schema, name); err != nil {
		t.Error("dropping non-existent queue should not error")
	}
}

func TestManagerGetNotFound(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	mgr := NewManager(pool)
	ctx := context.Background()

	schema := SchemaName("public")
	name := QueueName("nonexistent_queue_test")

	_, err := mgr.Get(ctx, schema, name)
	if err == nil {
		t.Fatal("Get() on non-existent queue should return error")
	}

	if _, ok := err.(*QueueNotFoundError); !ok {
		t.Errorf("Get() error type = %T, want *QueueNotFoundError", err)
	}
}
