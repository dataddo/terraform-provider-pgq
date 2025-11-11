package pgq

import "fmt"

// Custom error types - because errors are values, not strings
// This allows callers to make decisions based on error type

type (
	QueueError struct {
		Op    string // Operation that failed
		Queue FQN
		Err   error // Underlying error
	}

	QueueExistsError struct {
		Queue FQN
	}

	QueueNotFoundError struct {
		Queue FQN
	}

	PartmanError struct {
		Op    string
		Queue FQN
		Err   error
	}
)

func (e *QueueError) Error() string {
	return fmt.Sprintf("queue %s: %s failed: %v", e.Queue, e.Op, e.Err)
}

func (e *QueueError) Unwrap() error { return e.Err }

func (e *QueueExistsError) Error() string {
	return fmt.Sprintf("queue %s already exists", e.Queue)
}

func (e *QueueNotFoundError) Error() string {
	return fmt.Sprintf("queue %s not found", e.Queue)
}

func (e *PartmanError) Error() string {
	return fmt.Sprintf("pg_partman %s for %s: %v", e.Op, e.Queue, e.Err)
}

func (e *PartmanError) Unwrap() error { return e.Err }

// Helper to wrap errors with context
func wrapErr(op string, fqn FQN, err error) error {
	if err == nil {
		return nil
	}
	return &QueueError{Op: op, Queue: fqn, Err: err}
}

func wrapPartmanErr(op string, fqn FQN, err error) error {
	if err == nil {
		return nil
	}
	return &PartmanError{Op: op, Queue: fqn, Err: err}
}
