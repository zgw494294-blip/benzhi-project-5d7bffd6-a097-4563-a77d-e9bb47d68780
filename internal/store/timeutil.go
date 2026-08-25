package store

import (
	"database/sql"
	"time"
)

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

func parseNullableTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
