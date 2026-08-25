package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Repository struct{ db *sql.DB }

func Open(path string) (*Repository, error) {
	if path == "" {
		return nil, fmt.Errorf("数据库路径不能为空")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err = db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("配置 SQLite: %w", err)
		}
	}
	r := &Repository{db: db}
	if err = r.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }
