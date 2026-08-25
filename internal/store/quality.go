package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"groundwater-release/internal/domain"
)

func (s *TxStore) InsertQualityCheck(q domain.QualityCheck) error {
	results, err := json.Marshal(q.Results)
	if err != nil {
		return err
	}
	if _, err = s.tx.ExecContext(s.ctx, "UPDATE quality_exceptions SET current=0,invalidated_at=? WHERE campaign_id=? AND current=1", formatTime(q.CheckedAt), q.CampaignID); err != nil {
		return err
	}
	if _, err = s.tx.ExecContext(s.ctx, "UPDATE quality_checks SET current=0,invalidated_at=?,invalid_reason='被后续质量检查取代' WHERE campaign_id=? AND current=1", formatTime(q.CheckedAt), q.CampaignID); err != nil {
		return err
	}
	_, err = s.tx.ExecContext(s.ctx, `INSERT INTO quality_checks(id,campaign_id,rule_set_version,facts_revision,results,result_digest,checked_at,checked_by,current,invalidated_at,invalid_reason) VALUES(?,?,?,?,?,?,?,?,1,NULL,'')`, q.ID, q.CampaignID, q.RuleSetVersion, q.FactsRevision, results, q.ResultDigest, formatTime(q.CheckedAt), q.CheckedBy)
	if err != nil {
		return err
	}
	_, err = s.tx.ExecContext(s.ctx, `INSERT INTO campaign_current_checks(campaign_id,check_id) VALUES(?,?) ON CONFLICT(campaign_id) DO UPDATE SET check_id=excluded.check_id`, q.CampaignID, q.ID)
	return err
}

// ReplaceQualityCheck 保留旧内部调用名，但语义已升级为只追加。
func (s *TxStore) ReplaceQualityCheck(q domain.QualityCheck) error { return s.InsertQualityCheck(q) }

func scanCheck(row interface{ Scan(...any) error }) (domain.QualityCheck, error) {
	var q domain.QualityCheck
	var results, checked string
	var current int
	var invalid sql.NullString
	err := row.Scan(&q.ID, &q.CampaignID, &q.RuleSetVersion, &q.FactsRevision, &results, &q.ResultDigest, &checked, &q.CheckedBy, &current, &invalid, &q.InvalidReason)
	if err != nil {
		return q, err
	}
	if err = json.Unmarshal([]byte(results), &q.Results); err != nil {
		return q, err
	}
	if q.CheckedAt, err = parseTime(checked); err != nil {
		return q, err
	}
	q.Current = current == 1
	q.InvalidatedAt, err = parseNullableTime(invalid)
	return q, err
}

func (s *TxStore) LoadQualityCheck(campaignID string) (domain.QualityCheck, error) {
	row := s.tx.QueryRowContext(s.ctx, `SELECT q.id,q.campaign_id,q.rule_set_version,q.facts_revision,q.results,q.result_digest,q.checked_at,q.checked_by,q.current,q.invalidated_at,q.invalid_reason FROM quality_checks q JOIN campaign_current_checks p ON p.check_id=q.id WHERE p.campaign_id=?`, campaignID)
	q, err := scanCheck(row)
	if errors.Is(err, sql.ErrNoRows) {
		return q, domain.NewError(domain.ErrNotFound, "批次尚无当前有效质量检查")
	}
	return q, err
}

func (s *TxStore) LoadCheck(campaignID, checkID string) (domain.QualityCheck, error) {
	q, err := scanCheck(s.tx.QueryRowContext(s.ctx, `SELECT id,campaign_id,rule_set_version,facts_revision,results,result_digest,checked_at,checked_by,current,invalidated_at,invalid_reason FROM quality_checks WHERE campaign_id=? AND id=?`, campaignID, checkID))
	if errors.Is(err, sql.ErrNoRows) {
		return q, domain.NewError(domain.ErrNotFound, "质量检查不存在")
	}
	return q, err
}

func (s *TxStore) FindCheckDigest(campaignID, rule string, facts int64) (string, bool, error) {
	var digest string
	err := s.tx.QueryRowContext(s.ctx, `SELECT result_digest FROM quality_checks WHERE campaign_id=? AND rule_set_version=? AND facts_revision=? ORDER BY checked_at LIMIT 1`, campaignID, rule, facts).Scan(&digest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return digest, err == nil, err
}

func (s *TxStore) ListChecks(campaignID string, limit, offset int) ([]domain.QualityCheck, error) {
	rows, err := s.tx.QueryContext(s.ctx, `SELECT id,campaign_id,rule_set_version,facts_revision,results,result_digest,checked_at,checked_by,current,invalidated_at,invalid_reason FROM quality_checks WHERE campaign_id=? ORDER BY checked_at DESC,id DESC LIMIT ? OFFSET ?`, campaignID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.QualityCheck{}
	for rows.Next() {
		q, scanErr := scanCheck(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, q)
	}
	return items, rows.Err()
}

func (s *TxStore) CountChecks(campaignID string) (int, error) {
	var total int
	err := s.tx.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM quality_checks WHERE campaign_id=?", campaignID).Scan(&total)
	return total, err
}

func (s *TxStore) InvalidateCurrentCheck(campaignID, reason string, now time.Time) error {
	var checkID string
	err := s.tx.QueryRowContext(s.ctx, "SELECT check_id FROM campaign_current_checks WHERE campaign_id=?", campaignID).Scan(&checkID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewError(domain.ErrNotFound, "批次尚无当前有效质量检查")
	}
	if err != nil {
		return err
	}
	if _, err = s.tx.ExecContext(s.ctx, "UPDATE quality_checks SET current=0,invalidated_at=?,invalid_reason=? WHERE id=?", formatTime(now), reason, checkID); err != nil {
		return err
	}
	if _, err = s.tx.ExecContext(s.ctx, "UPDATE quality_exceptions SET current=0,invalidated_at=? WHERE check_id=?", formatTime(now), checkID); err != nil {
		return err
	}
	_, err = s.tx.ExecContext(s.ctx, "DELETE FROM campaign_current_checks WHERE campaign_id=?", campaignID)
	return err
}

func (s *TxStore) InsertException(e domain.QualityException) error { return s.saveException(e, true) }
func (s *TxStore) UpdateException(e domain.QualityException) error { return s.saveException(e, false) }

func (s *TxStore) saveException(e domain.QualityException, insert bool) error {
	evidence, err := json.Marshal(e.EvidenceRevisions)
	if err != nil {
		return err
	}
	query := `UPDATE quality_exceptions SET status=?,evidence_revisions=?,review_decision=?,review_comment=?,reviewed_by=?,reviewed_at=?,current=?,invalidated_at=? WHERE id=? AND campaign_id=?`
	args := []any{e.Status, evidence, e.ReviewDecision, e.ReviewComment, e.ReviewedBy, nullableTime(e.ReviewedAt), boolInt(e.Current), nullableTime(e.InvalidatedAt), e.ID, e.CampaignID}
	if insert {
		query = `INSERT INTO quality_exceptions(id,campaign_id,check_id,rule_code,subject_id,severity,description,status,evidence_revisions,review_decision,review_comment,reviewed_by,reviewed_at,current,invalidated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
		args = []any{e.ID, e.CampaignID, e.CheckID, e.RuleCode, e.SubjectID, e.Severity, e.Description, e.Status, evidence, e.ReviewDecision, e.ReviewComment, e.ReviewedBy, nullableTime(e.ReviewedAt), boolInt(e.Current), nullableTime(e.InvalidatedAt)}
	}
	res, err := s.tx.ExecContext(s.ctx, query, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return domain.NewError(domain.ErrNotFound, "质量异常不存在")
	}
	return nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *TxStore) LoadException(campaignID, id string) (domain.QualityException, error) {
	items, err := s.listExceptions(campaignID, " AND id=?", id)
	if err != nil {
		return domain.QualityException{}, err
	}
	if len(items) == 0 {
		return domain.QualityException{}, domain.NewError(domain.ErrNotFound, "质量异常不存在")
	}
	return items[0], nil
}
func (s *TxStore) ListExceptions(campaignID string) ([]domain.QualityException, error) {
	return s.listExceptions(campaignID, "")
}
func (s *TxStore) ListCurrentExceptions(campaignID string) ([]domain.QualityException, error) {
	return s.listExceptions(campaignID, " AND current=1")
}
func (s *TxStore) ListExceptionsByCheck(campaignID, checkID string) ([]domain.QualityException, error) {
	return s.listExceptions(campaignID, " AND check_id=?", checkID)
}

func (s *TxStore) listExceptions(campaignID, suffix string, args ...any) ([]domain.QualityException, error) {
	params := append([]any{campaignID}, args...)
	rows, err := s.tx.QueryContext(s.ctx, `SELECT id,campaign_id,check_id,rule_code,subject_id,severity,description,status,evidence_revisions,review_decision,review_comment,reviewed_by,reviewed_at,current,invalidated_at FROM quality_exceptions WHERE campaign_id=?`+suffix+` ORDER BY check_id,id`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.QualityException{}
	for rows.Next() {
		var e domain.QualityException
		var evidence string
		var reviewed, invalid sql.NullString
		var current int
		if err = rows.Scan(&e.ID, &e.CampaignID, &e.CheckID, &e.RuleCode, &e.SubjectID, &e.Severity, &e.Description, &e.Status, &evidence, &e.ReviewDecision, &e.ReviewComment, &e.ReviewedBy, &reviewed, &current, &invalid); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(evidence), &e.EvidenceRevisions); err != nil {
			return nil, err
		}
		e.ReviewedAt, err = parseNullableTime(reviewed)
		if err != nil {
			return nil, err
		}
		e.InvalidatedAt, err = parseNullableTime(invalid)
		if err != nil {
			return nil, err
		}
		e.Current = current == 1
		items = append(items, e)
	}
	return items, rows.Err()
}
