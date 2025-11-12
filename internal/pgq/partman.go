package pgq

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	undoPartitionBatchSize = 20
)

type PartitionConfig struct {
	Interval           string
	Premake            int
	Retention          string
	DatetimeString     string
	OptimizeConstraint int
	DefaultPartition   bool
}

func (m *Manager) CreatePartitioned(ctx context.Context, schema SchemaName, name QueueName, cfg *PartitionConfig) error {
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
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := m.createTable(ctx, tx, schema, name, true); err != nil {
		return err
	}

	if err := m.createIndexes(ctx, tx, schema, name); err != nil {
		return err
	}

	if err := m.createTemplate(ctx, tx, schema, name); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapErr("commit_ddl", fqn, err)
	}

	if err := m.setupPartman(ctx, schema, name, cfg); err != nil {
		return err
	}

	return nil
}

func (m *Manager) createTemplate(ctx context.Context, tx pgx.Tx, schema SchemaName, name QueueName) error {
	fqn := MakeFQN(schema, name)
	templateName := name.String() + "_template"

	var sql strings.Builder
	sql.WriteString("CREATE TABLE IF NOT EXISTS ")
	sql.WriteString(schema.Sanitize())
	sql.WriteString(".")
	sql.WriteString(pgx.Identifier{templateName}.Sanitize())
	sql.WriteString(" (LIKE ")
	sql.WriteString(schema.Sanitize())
	sql.WriteString(".")
	sql.WriteString(name.Sanitize())
	sql.WriteString(" INCLUDING ALL)")

	if _, err := tx.Exec(ctx, sql.String()); err != nil {
		return wrapErr("create_template", fqn, err)
	}

	return nil
}

func (m *Manager) setupPartman(ctx context.Context, schema SchemaName, name QueueName, cfg *PartitionConfig) error {
	fqn := MakeFQN(schema, name)
	parentTable := fqn.String()
	templateTable := fmt.Sprintf("%s.%s_template", schema, name)

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return wrapPartmanErr("begin_tx", fqn, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	_, err = tx.Exec(ctx, `
		SELECT partman.create_parent(
			p_parent_table          := $1,
			p_control               := $2,
			p_interval              := $3,
			p_type                  := $4,
			p_premake               := $5,
			p_start_partition       := $6,
			p_default_table         := $7,
			p_automatic_maintenance := $8,
			p_constraint_cols       := $9,
			p_template_table        := $10,
			p_jobmon                := $11
		)
	`, parentTable, "created_at", cfg.Interval, "range", cfg.Premake,
		nil, cfg.DefaultPartition, "on", nil, templateTable, true)

	if err != nil {
		return wrapPartmanErr("create_parent", fqn, err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE partman.part_config
		SET retention = $2,
		    retention_keep_index = TRUE,
		    retention_keep_table = FALSE,
		    datetime_string = $3,
		    optimize_constraint = $4,
		    ignore_default_data = TRUE
		WHERE parent_table = $1
	`, parentTable, cfg.Retention, cfg.DatetimeString, cfg.OptimizeConstraint)

	if err != nil {
		return wrapPartmanErr("update_config", fqn, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapPartmanErr("commit", fqn, err)
	}

	return nil
}

func (m *Manager) GetPartitionConfig(ctx context.Context, schema SchemaName, name QueueName) (*PartitionConfig, error) {
	fqn := MakeFQN(schema, name)

	var cfg PartitionConfig
	err := m.pool.QueryRow(ctx, `
		SELECT partition_interval::text, premake, retention::text,
		       datetime_string, optimize_constraint
		FROM partman.part_config
		WHERE parent_table = $1
	`, fqn.String()).Scan(
		&cfg.Interval, &cfg.Premake, &cfg.Retention,
		&cfg.DatetimeString, &cfg.OptimizeConstraint,
	)

	if err != nil {
		return nil, wrapPartmanErr("get_config", fqn, err)
	}

	// Check if default partition exists
	var hasDefault bool
	err = m.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_inherits i
			JOIN pg_class parent ON i.inhparent = parent.oid
			JOIN pg_class child ON i.inhrelid = child.oid
			JOIN pg_namespace n ON parent.relnamespace = n.oid
			WHERE n.nspname = $1
			  AND parent.relname = $2
			  AND child.relname LIKE '%_default'
		)
	`, schema, name).Scan(&hasDefault)

	if err != nil {
		return nil, wrapPartmanErr("check_default_partition", fqn, err)
	}

	cfg.DefaultPartition = hasDefault

	return &cfg, nil
}

func (m *Manager) UpdatePartitionConfig(ctx context.Context, schema SchemaName, name QueueName, cfg *PartitionConfig) error {
	fqn := MakeFQN(schema, name)

	_, err := m.pool.Exec(ctx, `
		UPDATE partman.part_config
		SET partition_interval = $2, premake = $3, retention = $4,
		    datetime_string = $5, optimize_constraint = $6
		WHERE parent_table = $1
	`, fqn.String(), cfg.Interval, cfg.Premake, cfg.Retention,
		cfg.DatetimeString, cfg.OptimizeConstraint)

	if err != nil {
		return wrapPartmanErr("update_config", fqn, err)
	}

	return nil
}

func (m *Manager) RemovePartmanConfig(ctx context.Context, schema SchemaName, name QueueName) error {
	fqn := MakeFQN(schema, name)

	_, err := m.pool.Exec(ctx, `SELECT partman.undo_partition($1, $2, p_keep_table := false)`, fqn.String(), undoPartitionBatchSize)
	if err != nil {
		return wrapPartmanErr("undo_partition", fqn, err)
	}

	return nil
}
