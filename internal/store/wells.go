package store

import (
	"database/sql"
	"encoding/json"

	"groundwater-release/internal/domain"
)

func (s *TxStore) InsertWell(w domain.MonitoringWell) error {
	analytes, err := json.Marshal(w.PlannedAnalytes)
	if err != nil {
		return err
	}
	_, err = s.tx.ExecContext(s.ctx, `INSERT INTO wells(id,campaign_id,well_code,location_label,planned_analytes,responsible_person,planned_sample_at) VALUES(?,?,?,?,?,?,?)`, w.ID, w.CampaignID, w.WellCode, w.LocationLabel, analytes, w.ResponsiblePerson, formatTime(w.PlannedSampleAt))
	return conflictOrContextErr(err, "监测井编号或 ID 已存在")
}

func (s *TxStore) WellExists(campaignID, wellID string) (bool, error) {
	var n int
	err := s.tx.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM wells WHERE campaign_id=? AND id=?", campaignID, wellID).Scan(&n)
	return n > 0, err
}

func (s *TxStore) ExistingWellCodes(campaignID string) (map[string]bool, error) {
	rows, err := s.tx.QueryContext(s.ctx, "SELECT UPPER(TRIM(well_code)) FROM wells WHERE campaign_id=?", campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]bool{}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			return nil, err
		}
		items[code] = true
	}
	return items, rows.Err()
}

func (s *TxStore) ListWells(campaignID string) ([]domain.MonitoringWell, error) {
	rows, err := s.tx.QueryContext(s.ctx, `SELECT id,campaign_id,well_code,location_label,planned_analytes,responsible_person,planned_sample_at FROM wells WHERE campaign_id=? ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.MonitoringWell{}
	for rows.Next() {
		var w domain.MonitoringWell
		var analytes, planned string
		if err = rows.Scan(&w.ID, &w.CampaignID, &w.WellCode, &w.LocationLabel, &analytes, &w.ResponsiblePerson, &planned); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(analytes), &w.PlannedAnalytes); err != nil {
			return nil, err
		}
		if w.PlannedSampleAt, err = parseTime(planned); err != nil {
			return nil, err
		}
		items = append(items, w)
	}
	return items, rows.Err()
}

var _ sql.Scanner
