package store

import (
	"database/sql"
	"errors"

	"groundwater-release/internal/domain"
)

func (s *TxStore) InsertCampaign(c domain.MonitoringCampaign) error {
	_, err := s.tx.ExecContext(s.ctx, `INSERT INTO campaigns(id,campaign_code,window_start,window_end,rule_set_version,status,version,facts_revision,created_at,approved_at,approved_check_id,approved_check_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.CampaignCode, formatTime(c.SamplingWindowStart), formatTime(c.SamplingWindowEnd), c.RuleSetVersion, c.Status, c.ExpectedVersion, c.FactsRevision, formatTime(c.CreatedAt), nullableTime(c.ApprovedAt), c.ApprovedCheckID, c.ApprovedCheckDigest)
	return conflictOrContextErr(err, "批次编号或 ID 已存在")
}

func (s *TxStore) LoadCampaign(id string) (domain.MonitoringCampaign, error) {
	var c domain.MonitoringCampaign
	var start, end, created string
	var approved sql.NullString
	err := s.tx.QueryRowContext(s.ctx, `SELECT id,campaign_code,window_start,window_end,rule_set_version,status,version,facts_revision,created_at,approved_at,approved_check_id,approved_check_digest FROM campaigns WHERE id=?`, id).Scan(&c.ID, &c.CampaignCode, &start, &end, &c.RuleSetVersion, &c.Status, &c.ExpectedVersion, &c.FactsRevision, &created, &approved, &c.ApprovedCheckID, &c.ApprovedCheckDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.NewError(domain.ErrNotFound, "批次不存在")
	}
	if err != nil {
		return c, err
	}
	if c.SamplingWindowStart, err = parseTime(start); err != nil {
		return c, err
	}
	if c.SamplingWindowEnd, err = parseTime(end); err != nil {
		return c, err
	}
	if c.CreatedAt, err = parseTime(created); err != nil {
		return c, err
	}
	c.ApprovedAt, err = parseNullableTime(approved)
	return c, err
}

func (s *TxStore) SaveCampaign(c domain.MonitoringCampaign, previousVersion int64) error {
	res, err := s.tx.ExecContext(s.ctx, `UPDATE campaigns SET status=?,version=?,facts_revision=?,approved_at=?,approved_check_id=?,approved_check_digest=? WHERE id=? AND version=?`, c.Status, c.ExpectedVersion, c.FactsRevision, nullableTime(c.ApprovedAt), c.ApprovedCheckID, c.ApprovedCheckDigest, c.ID, previousVersion)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.NewError(domain.ErrStaleVersion, "批次版本已被其他请求更新")
	}
	return nil
}
