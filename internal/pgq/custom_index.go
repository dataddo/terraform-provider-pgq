package pgq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	maxColumnNameLength = 20
	hashLength          = 8

	// Default pgq index suffixes
	indexCreatedAt       = "_created_at_idx"
	indexProcessedAtNull = "_processed_at_null_idx"
	indexScheduledFor    = "_scheduled_for_idx"
	indexMetadata        = "_metadata_idx"
)

type CustomIndex struct {
	Name    string
	Columns []string
	Type    string
	Where   string
}

func (m *Manager) CreateCustomIndexes(ctx context.Context, tx pgx.Tx, schema SchemaName, name QueueName, indexes []CustomIndex) error {
	fqn := MakeFQN(schema, name)

	for _, idx := range indexes {
		indexName := idx.Name
		if indexName == "" {
			indexName = generateIndexName(name.String(), idx.Columns, idx.Type)
		}

		var sql strings.Builder
		sql.WriteString("CREATE INDEX IF NOT EXISTS ")
		sql.WriteString(pgx.Identifier{indexName}.Sanitize())
		sql.WriteString(" ON ")
		sql.WriteString(schema.Sanitize())
		sql.WriteString(".")
		sql.WriteString(name.Sanitize())

		if idx.Type != "" && idx.Type != "btree" {
			sql.WriteString(" USING ")
			sql.WriteString(idx.Type)
		}

		sql.WriteString(" (")
		for i, col := range idx.Columns {
			if i > 0 {
				sql.WriteString(", ")
			}
			sql.WriteString(col)
		}
		sql.WriteString(")")

		if idx.Where != "" {
			sql.WriteString(" WHERE ")
			sql.WriteString(idx.Where)
		}

		if _, err := tx.Exec(ctx, sql.String()); err != nil {
			return wrapErr("create_custom_index_"+indexName, fqn, err)
		}
	}

	return nil
}

func (m *Manager) GetCustomIndexes(ctx context.Context, schema SchemaName, name QueueName) ([]CustomIndex, error) {
	fqn := MakeFQN(schema, name)

	rows, err := m.pool.Query(ctx, `
		SELECT
			i.relname AS index_name,
			pg_get_indexdef(i.oid) AS index_def
		FROM pg_index x
		JOIN pg_class t ON t.oid = x.indrelid
		JOIN pg_class i ON i.oid = x.indexrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = $1
		  AND t.relname = $2
		  AND i.relname NOT LIKE '%_pkey'
		  AND i.relname NOT IN (
		      $3, $4, $5, $6
		  )
		ORDER BY i.relname
	`, schema, name,
		name.String()+indexCreatedAt,
		name.String()+indexProcessedAtNull,
		name.String()+indexScheduledFor,
		name.String()+indexMetadata)

	if err != nil {
		return nil, wrapErr("get_custom_indexes", fqn, err)
	}
	defer rows.Close()

	var indexes []CustomIndex
	for rows.Next() {
		var indexName, indexDef string
		if err := rows.Scan(&indexName, &indexDef); err != nil {
			return nil, wrapErr("scan_custom_index", fqn, err)
		}

		idx := parseIndexDef(indexName, indexDef)
		indexes = append(indexes, idx)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapErr("get_custom_indexes_rows", fqn, err)
	}

	return indexes, nil
}

func (m *Manager) DropCustomIndexes(ctx context.Context, schema SchemaName, name QueueName, indexNames []string) error {
	fqn := MakeFQN(schema, name)

	for _, indexName := range indexNames {
		sql := fmt.Sprintf("DROP INDEX IF EXISTS %s.%s",
			schema.Sanitize(),
			pgx.Identifier{indexName}.Sanitize())

		if _, err := m.pool.Exec(ctx, sql); err != nil {
			return wrapErr("drop_custom_index_"+indexName, fqn, err)
		}
	}

	return nil
}

func generateIndexName(tableName string, columns []string, indexType string) string {
	// Use strings.Replacer for efficient multiple replacements
	replacer := strings.NewReplacer(
		"(", "",
		")", "",
		"'", "",
		"\"", "",
		"->>", "_",
		"->", "_",
		" ", "_",
	)

	parts := []string{tableName}
	for _, col := range columns {
		clean := replacer.Replace(col)
		if len(clean) > maxColumnNameLength {
			clean = clean[:maxColumnNameLength]
		}
		parts = append(parts, clean)
	}

	baseName := strings.Join(parts, "_")

	if indexType != "" && indexType != "btree" {
		baseName += "_" + indexType
	}

	hash := sha256.Sum256([]byte(strings.Join(columns, ",")))
	baseName += "_" + hex.EncodeToString(hash[:])[:hashLength]
	baseName += "_idx"

	if len(baseName) > maxIdentifierLength {
		baseName = baseName[:maxIdentifierLength]
	}

	return baseName
}

func parseIndexDef(name, def string) CustomIndex {
	idx := CustomIndex{Name: name}

	if strings.Contains(def, " USING gin ") {
		idx.Type = "gin"
	} else if strings.Contains(def, " USING gist ") {
		idx.Type = "gist"
	} else if strings.Contains(def, " USING hash ") {
		idx.Type = "hash"
	} else if strings.Contains(def, " USING brin ") {
		idx.Type = "brin"
	} else {
		idx.Type = "btree"
	}

	columnsStart := strings.Index(def, "(")
	columnsEnd := strings.LastIndex(def, ")")
	if columnsStart != -1 && columnsEnd != -1 && columnsEnd > columnsStart {
		columnsStr := def[columnsStart+1 : columnsEnd]
		if whereIdx := strings.Index(strings.ToUpper(columnsStr), " WHERE "); whereIdx != -1 {
			columnsStr = columnsStr[:whereIdx]
		}
		idx.Columns = strings.Split(columnsStr, ", ")
	}

	if whereIdx := strings.Index(strings.ToUpper(def), " WHERE "); whereIdx != -1 {
		idx.Where = strings.TrimSpace(def[whereIdx+7:])
	}

	return idx
}
