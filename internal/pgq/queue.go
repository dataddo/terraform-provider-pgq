package pgq

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Manager struct {
	pool *pgxpool.Pool
}

func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool}
}

func (m *Manager) CreateSimple(ctx context.Context, schema SchemaName, name QueueName) error {
	fqn := MakeFQN(schema, name)

	exists, err := m.Exists(ctx, schema, name)
	if err != nil {
		return err
	}
	if exists {
		return &QueueExistsError{Queue: fqn}
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return wrapErr("begin_tx", fqn, err)
	}
	defer tx.Rollback(ctx)

	if err := m.createTable(ctx, tx, schema, name, false); err != nil {
		return err
	}

	if err := m.createIndexes(ctx, tx, schema, name); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapErr("commit", fqn, err)
	}

	return nil
}

func (m *Manager) createTable(ctx context.Context, tx pgx.Tx, schema SchemaName, name QueueName, partitioned bool) error {
	fqn := MakeFQN(schema, name)

	var sql strings.Builder
	sql.WriteString("CREATE TABLE IF NOT EXISTS ")
	sql.WriteString(schema.Sanitize())
	sql.WriteString(".")
	sql.WriteString(name.Sanitize())
	sql.WriteString(` (
		id             UUID        NOT NULL DEFAULT gen_random_uuid(),
		created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at     TIMESTAMPTZ,
		locked_until   TIMESTAMPTZ,
		scheduled_for  TIMESTAMPTZ,
		processed_at   TIMESTAMPTZ,
		consumed_count INTEGER     NOT NULL DEFAULT 0,
		error_detail   TEXT,
		payload        JSONB       NOT NULL,
		metadata       JSONB       NOT NULL,
		`)

	if partitioned {
		sql.WriteString("PRIMARY KEY (id, created_at)")
		sql.WriteString(") PARTITION BY RANGE (created_at)")
	} else {
		sql.WriteString("PRIMARY KEY (id)")
		sql.WriteString(")")
	}

	if _, err := tx.Exec(ctx, sql.String()); err != nil {
		return wrapErr("create_table", fqn, err)
	}

	return nil
}

func (m *Manager) createIndexes(ctx context.Context, tx pgx.Tx, schema SchemaName, name QueueName) error {
	fqn := MakeFQN(schema, name)

	indexes := []struct {
		suffix string
		def    string
	}{
		{indexCreatedAt, "(created_at)"},
		{indexProcessedAtNull, "(processed_at) WHERE (processed_at IS NULL)"},
		{indexScheduledFor, "(scheduled_for ASC NULLS LAST) WHERE (processed_at IS NULL)"},
		{indexMetadata, "USING GIN(metadata) WHERE processed_at IS NULL"},
	}

	for _, idx := range indexes {
		var sql strings.Builder
		sql.WriteString("CREATE INDEX IF NOT EXISTS ")
		sql.WriteString(pgx.Identifier{name.String() + idx.suffix}.Sanitize())
		sql.WriteString(" ON ")
		sql.WriteString(schema.Sanitize())
		sql.WriteString(".")
		sql.WriteString(name.Sanitize())
		sql.WriteString(" ")
		sql.WriteString(idx.def)

		if _, err := tx.Exec(ctx, sql.String()); err != nil {
			return wrapErr("create_index"+idx.suffix, fqn, err)
		}
	}

	return nil
}

// Exists checks if a queue table exists
func (m *Manager) Exists(ctx context.Context, schema SchemaName, name QueueName) (bool, error) {
	fqn := MakeFQN(schema, name)

	var exists bool
	err := m.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables
			WHERE schemaname = $1 AND tablename = $2
		)
	`, schema, name).Scan(&exists)

	if err != nil {
		return false, wrapErr("check_exists", fqn, err)
	}

	return exists, nil
}

// IsPartitioned checks if a queue uses partitioning
func (m *Manager) IsPartitioned(ctx context.Context, schema SchemaName, name QueueName) (bool, error) {
	fqn := MakeFQN(schema, name)

	var partitioned bool
	err := m.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_partitioned_table pt
			JOIN pg_class c ON pt.partrelid = c.oid
			JOIN pg_namespace n ON c.relnamespace = n.oid
			WHERE n.nspname = $1 AND c.relname = $2
		)
	`, schema, name).Scan(&partitioned)

	if err != nil {
		return false, wrapErr("check_partitioned", fqn, err)
	}

	return partitioned, nil
}

// Get retrieves queue information
func (m *Manager) Get(ctx context.Context, schema SchemaName, name QueueName) (*Queue, error) {
	fqn := MakeFQN(schema, name)

	exists, err := m.Exists(ctx, schema, name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &QueueNotFoundError{Queue: fqn}
	}

	partitioned, err := m.IsPartitioned(ctx, schema, name)
	if err != nil {
		return nil, err
	}

	return &Queue{
		Name:        name,
		Schema:      schema,
		Partitioned: partitioned,
	}, nil
}

// Drop removes a queue table entirely
// This is destructive - caller should confirm
func (m *Manager) Drop(ctx context.Context, schema SchemaName, name QueueName) error {
	fqn := MakeFQN(schema, name)

	sql := strings.Builder{}
	sql.WriteString("DROP TABLE IF EXISTS ")
	sql.WriteString(schema.Sanitize())
	sql.WriteString(".")
	sql.WriteString(name.Sanitize())
	sql.WriteString(" CASCADE")

	if _, err := m.pool.Exec(ctx, sql.String()); err != nil {
		return wrapErr("drop", fqn, err)
	}

	return nil
}

// Pool returns the underlying connection pool
// Sometimes you need raw access - don't hide it
func (m *Manager) Pool() *pgxpool.Pool {
	return m.pool
}
