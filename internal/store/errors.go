package store

import (
	"context"
	"errors"
	"strings"

	"groundwater-release/internal/domain"
	"modernc.org/sqlite"
)

// sqliteConstraintPrimaryCode is the SQLite primary result code for
// SQLITE_CONSTRAINT. Extended constraint codes share the lower byte.
const sqliteConstraintPrimaryCode = 19

// conflictOrContextErr maps a SQLite write error to a domain conflict error
// only when the error is a genuine constraint violation. Context
// cancellation and deadline errors are returned unchanged so that callers
// can identify them via errors.Is(err, context.Canceled) or
// errors.Is(err, context.DeadlineExceeded). All other non-constraint
// errors are also returned as-is.
func conflictOrContextErr(err error, conflictMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if isConstraintError(err) {
		return domain.WrapConflict(conflictMsg)
	}
	return err
}

// isConstraintError reports whether err originates from a SQLite
// constraint violation.
func isConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()&0xFF == sqliteConstraintPrimaryCode
	}
	msg := err.Error()
	return strings.Contains(msg, "constraint failed") || strings.Contains(msg, "UNIQUE constraint")
}
