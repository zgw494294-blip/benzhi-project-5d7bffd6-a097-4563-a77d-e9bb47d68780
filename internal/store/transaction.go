package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"groundwater-release/internal/domain"
)

type TxStore struct {
	tx  *sql.Tx
	ctx context.Context
}

type IdempotentResult struct {
	Response json.RawMessage
	Replayed bool
}

func (r *Repository) Execute(ctx context.Context, key, operation string, fn func(*TxStore) (json.RawMessage, error)) (IdempotentResult, error) {
	if key == "" {
		return IdempotentResult{}, domain.FieldError("idempotencyKey", "idempotencyKey 不能为空")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return IdempotentResult{}, err
	}
	defer tx.Rollback()
	var storedOperation string
	var response []byte
	err = tx.QueryRowContext(ctx, "SELECT operation,response FROM idempotency_records WHERE idempotency_key=?", key).Scan(&storedOperation, &response)
	if err == nil {
		if storedOperation != operation {
			return IdempotentResult{}, domain.WrapConflict("idempotencyKey 已用于其他操作")
		}
		return IdempotentResult{Response: response, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return IdempotentResult{}, err
	}
	ts := &TxStore{tx: tx, ctx: ctx}
	response, err = fn(ts)
	if err != nil {
		return IdempotentResult{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO idempotency_records(idempotency_key,operation,response,created_at) VALUES(?,?,?,?)", key, operation, response, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return IdempotentResult{}, conflictOrContextErr(err, "idempotencyKey 已存在")
	}
	if err = tx.Commit(); err != nil {
		return IdempotentResult{}, err
	}
	return IdempotentResult{Response: response}, nil
}

func (r *Repository) View(ctx context.Context, fn func(*TxStore) error) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&TxStore{tx: tx, ctx: ctx}); err != nil {
		return err
	}
	return tx.Commit()
}
