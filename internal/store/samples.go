package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"groundwater-release/internal/domain"
)

func scanSample(row interface{ Scan(...any) error }) (domain.SampleRecord, error) {
	var sample domain.SampleRecord
	var collected, expires, measurements, custody string
	err := row.Scan(&sample.ID, &sample.CampaignID, &sample.WellID, &sample.SampleCode, &sample.SampleKind, &collected, &measurements, &sample.PreservationMethod, &expires, &custody, &sample.Revision)
	if err != nil {
		return sample, err
	}
	if sample.CollectedAt, err = parseTime(collected); err != nil {
		return sample, err
	}
	if sample.PreservationExpiresAt, err = parseTime(expires); err != nil {
		return sample, err
	}
	if err = json.Unmarshal([]byte(measurements), &sample.FieldMeasurements); err != nil {
		return sample, err
	}
	if err = json.Unmarshal([]byte(custody), &sample.CustodyEvents); err != nil {
		return sample, err
	}
	return sample, nil
}

func (s *TxStore) LoadSample(campaignID, sampleID string) (domain.SampleRecord, error) {
	row := s.tx.QueryRowContext(s.ctx, `SELECT id,campaign_id,COALESCE(well_id,''),sample_code,sample_kind,collected_at,field_measurements,preservation_method,preservation_expires_at,custody_events,revision FROM samples WHERE campaign_id=? AND id=?`, campaignID, sampleID)
	sample, err := scanSample(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sample, domain.NewError(domain.ErrNotFound, "样品不存在")
	}
	return sample, err
}

func (s *TxStore) ReviseSample(before, after domain.SampleRecord, actor string, now time.Time) error {
	snapshot, err := json.Marshal(before)
	if err != nil {
		return err
	}
	if _, err = s.tx.ExecContext(s.ctx, `INSERT INTO sample_revision_history(sample_id,revision,snapshot,revised_at,revised_by) VALUES(?,?,?,?,?)`, before.ID, before.Revision, snapshot, formatTime(now), actor); err != nil {
		return err
	}
	measurements, err := json.Marshal(after.FieldMeasurements)
	if err != nil {
		return err
	}
	custody, err := json.Marshal(after.CustodyEvents)
	if err != nil {
		return err
	}
	res, err := s.tx.ExecContext(s.ctx, `UPDATE samples SET field_measurements=?,preservation_method=?,preservation_expires_at=?,custody_events=?,revision=? WHERE id=? AND campaign_id=? AND revision=?`, measurements, after.PreservationMethod, formatTime(after.PreservationExpiresAt), custody, after.Revision, after.ID, after.CampaignID, before.Revision)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return domain.NewError(domain.ErrStaleVersion, "样品 revision 已被其他请求更新")
	}
	return nil
}

func (s *TxStore) ListSampleHistory(sampleID string) ([]domain.SampleRevisionSnapshot, error) {
	rows, err := s.tx.QueryContext(s.ctx, `SELECT snapshot,revised_at,revised_by FROM sample_revision_history WHERE sample_id=? ORDER BY revision`, sampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SampleRevisionSnapshot{}
	for rows.Next() {
		var raw, at, by string
		if err = rows.Scan(&raw, &at, &by); err != nil {
			return nil, err
		}
		var item domain.SampleRevisionSnapshot
		if err = json.Unmarshal([]byte(raw), &item.Sample); err != nil {
			return nil, err
		}
		if item.RevisedAt, err = parseTime(at); err != nil {
			return nil, err
		}
		item.RevisedBy = by
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *TxStore) InsertSample(sample domain.SampleRecord) error {
	measurements, err := json.Marshal(sample.FieldMeasurements)
	if err != nil {
		return err
	}
	custody, err := json.Marshal(sample.CustodyEvents)
	if err != nil {
		return err
	}
	_, err = s.tx.ExecContext(s.ctx, `INSERT INTO samples(id,campaign_id,well_id,sample_code,sample_kind,collected_at,field_measurements,preservation_method,preservation_expires_at,custody_events,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sample.ID, sample.CampaignID, nullString(sample.WellID), sample.SampleCode, sample.SampleKind, formatTime(sample.CollectedAt), measurements, sample.PreservationMethod, formatTime(sample.PreservationExpiresAt), custody, sample.Revision)
	if err != nil {
		return domain.WrapConflict("样品编号或 ID 已存在")
	}
	return nil
}

func (s *TxStore) ListSamples(campaignID string) ([]domain.SampleRecord, error) {
	rows, err := s.tx.QueryContext(s.ctx, `SELECT id,campaign_id,COALESCE(well_id,''),sample_code,sample_kind,collected_at,field_measurements,preservation_method,preservation_expires_at,custody_events,revision FROM samples WHERE campaign_id=? ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SampleRecord{}
	for rows.Next() {
		sample, scanErr := scanSample(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, sample)
	}
	return items, rows.Err()
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
