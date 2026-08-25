package store

import (
	"context"
	"fmt"
)

const schemaVersion = 2

var baseSchema = []string{
	`CREATE TABLE IF NOT EXISTS campaigns (
		id TEXT PRIMARY KEY, campaign_code TEXT NOT NULL UNIQUE,
		window_start TEXT NOT NULL, window_end TEXT NOT NULL, rule_set_version TEXT NOT NULL,
		status TEXT NOT NULL, version INTEGER NOT NULL, facts_revision INTEGER NOT NULL,
		created_at TEXT NOT NULL, approved_at TEXT, approved_check_id TEXT NOT NULL DEFAULT '',
		approved_check_digest TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS wells (
		id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id), well_code TEXT NOT NULL,
		location_label TEXT NOT NULL, planned_analytes TEXT NOT NULL, responsible_person TEXT NOT NULL,
		planned_sample_at TEXT NOT NULL, UNIQUE(campaign_id, well_code)
	);`,
	`CREATE TABLE IF NOT EXISTS samples (
		id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id), well_id TEXT,
		sample_code TEXT NOT NULL UNIQUE, sample_kind TEXT NOT NULL, collected_at TEXT NOT NULL,
		field_measurements TEXT NOT NULL, preservation_method TEXT NOT NULL, preservation_expires_at TEXT NOT NULL,
		custody_events TEXT NOT NULL, revision INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS sample_revision_history (
		sample_id TEXT NOT NULL REFERENCES samples(id), revision INTEGER NOT NULL, snapshot TEXT NOT NULL,
		revised_at TEXT NOT NULL, revised_by TEXT NOT NULL, PRIMARY KEY(sample_id,revision)
	);`,
	`CREATE TABLE IF NOT EXISTS quality_checks (
		id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id), rule_set_version TEXT NOT NULL,
		facts_revision INTEGER NOT NULL, results TEXT NOT NULL, result_digest TEXT NOT NULL,
		checked_at TEXT NOT NULL, checked_by TEXT NOT NULL, current INTEGER NOT NULL,
		invalidated_at TEXT, invalid_reason TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS campaign_current_checks (
		campaign_id TEXT PRIMARY KEY REFERENCES campaigns(id), check_id TEXT NOT NULL UNIQUE REFERENCES quality_checks(id)
	);`,
	`CREATE TABLE IF NOT EXISTS quality_exceptions (
		id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id), check_id TEXT NOT NULL,
		rule_code TEXT NOT NULL, subject_id TEXT NOT NULL, severity TEXT NOT NULL, description TEXT NOT NULL,
		status TEXT NOT NULL, evidence_revisions TEXT NOT NULL, review_decision TEXT NOT NULL,
		review_comment TEXT NOT NULL, reviewed_by TEXT NOT NULL, reviewed_at TEXT,
		current INTEGER NOT NULL, invalidated_at TEXT, UNIQUE(check_id,rule_code,subject_id)
	);`,
	`CREATE TABLE IF NOT EXISTS frozen_datasets (
		campaign_id TEXT PRIMARY KEY REFERENCES campaigns(id), dataset_version INTEGER NOT NULL,
		content BLOB NOT NULL, digest TEXT NOT NULL UNIQUE, frozen_at TEXT NOT NULL, frozen_by TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS credentials (
		serial_number INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
		campaign_id TEXT NOT NULL UNIQUE REFERENCES campaigns(id), dataset_version INTEGER NOT NULL,
		dataset_digest TEXT NOT NULL, previous_digest TEXT NOT NULL, credential_digest TEXT NOT NULL UNIQUE,
		issued_at TEXT NOT NULL, issued_by TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS audit_events (
		campaign_id TEXT NOT NULL REFERENCES campaigns(id), sequence INTEGER NOT NULL,
		event_type TEXT NOT NULL, actor TEXT NOT NULL, occurred_at TEXT NOT NULL, payload BLOB NOT NULL,
		previous_digest TEXT NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(campaign_id,sequence), UNIQUE(digest)
	);`,
	`CREATE TABLE IF NOT EXISTS idempotency_records (
		idempotency_key TEXT PRIMARY KEY, operation TEXT NOT NULL, response BLOB NOT NULL, created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_wells_campaign ON wells(campaign_id);`,
	`CREATE INDEX IF NOT EXISTS idx_samples_campaign ON samples(campaign_id);`,
	`CREATE INDEX IF NOT EXISTS idx_checks_campaign ON quality_checks(campaign_id,checked_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_exceptions_campaign ON quality_exceptions(campaign_id,check_id);`,
	`CREATE INDEX IF NOT EXISTS idx_audit_campaign ON audit_events(campaign_id,sequence);`,
}

var migrationV2 = []string{
	`ALTER TABLE campaigns ADD COLUMN approved_check_id TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE campaigns ADD COLUMN approved_check_digest TEXT NOT NULL DEFAULT '';`,
	`CREATE TABLE sample_revision_history (sample_id TEXT NOT NULL REFERENCES samples(id), revision INTEGER NOT NULL, snapshot TEXT NOT NULL, revised_at TEXT NOT NULL, revised_by TEXT NOT NULL, PRIMARY KEY(sample_id,revision));`,
	`ALTER TABLE quality_checks RENAME TO quality_checks_v1;`,
	`CREATE TABLE quality_checks (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id), rule_set_version TEXT NOT NULL, facts_revision INTEGER NOT NULL, results TEXT NOT NULL, result_digest TEXT NOT NULL, checked_at TEXT NOT NULL, checked_by TEXT NOT NULL, current INTEGER NOT NULL, invalidated_at TEXT, invalid_reason TEXT NOT NULL DEFAULT '');`,
	`INSERT INTO quality_checks SELECT id,campaign_id,rule_set_version,facts_revision,results,result_digest,checked_at,checked_by,1,NULL,'' FROM quality_checks_v1;`,
	`CREATE TABLE campaign_current_checks (campaign_id TEXT PRIMARY KEY REFERENCES campaigns(id), check_id TEXT NOT NULL UNIQUE REFERENCES quality_checks(id));`,
	`INSERT INTO campaign_current_checks SELECT campaign_id,id FROM quality_checks;`,
	`DROP TABLE quality_checks_v1;`,
	`ALTER TABLE quality_exceptions RENAME TO quality_exceptions_v1;`,
	`CREATE TABLE quality_exceptions (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id), check_id TEXT NOT NULL, rule_code TEXT NOT NULL, subject_id TEXT NOT NULL, severity TEXT NOT NULL, description TEXT NOT NULL, status TEXT NOT NULL, evidence_revisions TEXT NOT NULL, review_decision TEXT NOT NULL, review_comment TEXT NOT NULL, reviewed_by TEXT NOT NULL, reviewed_at TEXT, current INTEGER NOT NULL, invalidated_at TEXT, UNIQUE(check_id,rule_code,subject_id));`,
	`INSERT INTO quality_exceptions SELECT id,campaign_id,check_id,rule_code,subject_id,severity,description,status,evidence_revisions,review_decision,review_comment,reviewed_by,reviewed_at,1,NULL FROM quality_exceptions_v1;`,
	`DROP TABLE quality_exceptions_v1;`,
}

func (r *Repository) migrate(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER NOT NULL);`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO schema_meta(version) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM schema_meta);`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRowContext(ctx, "SELECT version FROM schema_meta").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("数据库版本 %d 高于程序版本 %d", version, schemaVersion)
	}
	if version == 1 {
		for i, statement := range migrationV2 {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("执行 v2 迁移 %d: %w", i+1, err)
			}
		}
	}
	for i, statement := range baseSchema {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("建立结构 %d: %w", i+1, err)
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE schema_meta SET version=?", schemaVersion); err != nil {
		return err
	}
	return tx.Commit()
}
