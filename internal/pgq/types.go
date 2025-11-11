package pgq

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	// PostgreSQL identifier length limit
	maxIdentifierLength = 63
)

// Domain types - wrap primitives for type safety and domain clarity
// This prevents mixing up queue names with schema names, etc.
type (
	QueueName  string
	SchemaName string
	FQN        string // Fully Qualified Name
)

// String returns the string representation
func (q QueueName) String() string  { return string(q) }
func (s SchemaName) String() string { return string(s) }
func (f FQN) String() string        { return string(f) }

// Valid checks if the name is a valid PostgreSQL identifier
func (q QueueName) Valid() bool {
	if q == "" {
		return false
	}
	s := string(q)
	if len(s) > maxIdentifierLength {
		return false
	}
	// Must start with letter or underscore
	first := s[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}
	return true
}

func (s SchemaName) Valid() bool { return QueueName(s).Valid() }

// Sanitize returns a safely quoted identifier for use in SQL
func (q QueueName) Sanitize() string  { return pgx.Identifier{q.String()}.Sanitize() }
func (s SchemaName) Sanitize() string { return pgx.Identifier{s.String()}.Sanitize() }

// FQN creates a fully qualified name from schema and queue
func MakeFQN(schema SchemaName, queue QueueName) FQN {
	return FQN(fmt.Sprintf("%s.%s", schema, queue))
}

// Split breaks an FQN into schema and queue name
func (f FQN) Split() (SchemaName, QueueName, error) {
	parts := strings.SplitN(string(f), ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid FQN format: %s (expected schema.queue)", f)
	}
	return SchemaName(parts[0]), QueueName(parts[1]), nil
}

// Queue represents a pgq queue - keep it simple, stupid
type Queue struct {
	Name        QueueName
	Schema      SchemaName
	Partitioned bool
}

// FQN returns the fully qualified name
func (q *Queue) FQN() FQN {
	return MakeFQN(q.Schema, q.Name)
}

// TemplateName returns the template table name for partitioned queues
func (q *Queue) TemplateName() QueueName {
	return QueueName(string(q.Name) + "_template")
}

// TemplateFQN returns the fully qualified template table name
func (q *Queue) TemplateFQN() FQN {
	return MakeFQN(q.Schema, q.TemplateName())
}
