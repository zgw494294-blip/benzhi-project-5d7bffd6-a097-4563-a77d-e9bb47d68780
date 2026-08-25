package store

import (
	"database/sql"
	"errors"

	"groundwater-release/internal/domain"
)

func (s *TxStore) InsertDataset(d domain.FrozenDataset) error {
	_, err := s.tx.ExecContext(s.ctx, `INSERT INTO frozen_datasets(campaign_id,dataset_version,content,digest,frozen_at,frozen_by) VALUES(?,?,?,?,?,?)`, d.CampaignID, d.DatasetVersion, d.Content, d.Digest, formatTime(d.FrozenAt), d.FrozenBy)
	if err != nil {
		return domain.WrapConflict("冻结数据集已存在，不能覆盖")
	}
	return nil
}

func (s *TxStore) LoadDataset(campaignID string) (domain.FrozenDataset, error) {
	var d domain.FrozenDataset
	var frozen string
	err := s.tx.QueryRowContext(s.ctx, `SELECT campaign_id,dataset_version,content,digest,frozen_at,frozen_by FROM frozen_datasets WHERE campaign_id=?`, campaignID).Scan(&d.CampaignID, &d.DatasetVersion, &d.Content, &d.Digest, &frozen, &d.FrozenBy)
	if errors.Is(err, sql.ErrNoRows) {
		return d, domain.NewError(domain.ErrNotFound, "冻结数据集不存在")
	}
	if err != nil {
		return d, err
	}
	d.FrozenAt, err = parseTime(frozen)
	return d, err
}

func (s *TxStore) NextCredentialSerial() (int64, string, error) {
	var next int64
	var previous string
	err := s.tx.QueryRowContext(s.ctx, "SELECT COALESCE(MAX(serial_number),0)+1 FROM credentials").Scan(&next)
	if err != nil {
		return 0, "", err
	}
	err = s.tx.QueryRowContext(s.ctx, "SELECT credential_digest FROM credentials ORDER BY serial_number DESC LIMIT 1").Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		return next, "", nil
	}
	return next, previous, err
}

func (s *TxStore) InsertCredential(c domain.ReleaseCredential) error {
	_, err := s.tx.ExecContext(s.ctx, `INSERT INTO credentials(serial_number,id,campaign_id,dataset_version,dataset_digest,previous_digest,credential_digest,issued_at,issued_by) VALUES(?,?,?,?,?,?,?,?,?)`, c.SerialNumber, c.ID, c.CampaignID, c.DatasetVersion, c.DatasetDigest, c.PreviousDigest, c.CredentialDigest, formatTime(c.IssuedAt), c.IssuedBy)
	if err != nil {
		return domain.WrapConflict("该冻结数据集已签发凭据，不能覆盖")
	}
	return nil
}

func (s *TxStore) LoadCredential(campaignID string) (*domain.ReleaseCredential, error) {
	var c domain.ReleaseCredential
	var issued string
	err := s.tx.QueryRowContext(s.ctx, `SELECT id,campaign_id,serial_number,dataset_version,dataset_digest,previous_digest,credential_digest,issued_at,issued_by FROM credentials WHERE campaign_id=?`, campaignID).Scan(&c.ID, &c.CampaignID, &c.SerialNumber, &c.DatasetVersion, &c.DatasetDigest, &c.PreviousDigest, &c.CredentialDigest, &issued, &c.IssuedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IssuedAt, err = parseTime(issued)
	return &c, err
}

func (s *TxStore) PreviousCredential(serial int64) (*domain.ReleaseCredential, error) {
	if serial <= 1 {
		return nil, nil
	}
	var c domain.ReleaseCredential
	var issued string
	stmt, err := s.repo.previousCredentialStatement(s.ctx, s.tx)
	if err != nil {
		return nil, err
	}
	err = stmt.QueryRowContext(s.ctx, serial-1).Scan(&c.ID, &c.CampaignID, &c.SerialNumber, &c.DatasetVersion, &c.DatasetDigest, &c.PreviousDigest, &c.CredentialDigest, &issued, &c.IssuedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IssuedAt, err = parseTime(issued)
	return &c, err
}

func (s *TxStore) ListCredentialsThrough(serial int64) ([]domain.ReleaseCredential, error) {
	rows, err := s.tx.QueryContext(s.ctx, `SELECT id,campaign_id,serial_number,dataset_version,dataset_digest,previous_digest,credential_digest,issued_at,issued_by FROM credentials WHERE serial_number<=? ORDER BY serial_number`, serial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ReleaseCredential{}
	for rows.Next() {
		var c domain.ReleaseCredential
		var issued string
		if err = rows.Scan(&c.ID, &c.CampaignID, &c.SerialNumber, &c.DatasetVersion, &c.DatasetDigest, &c.PreviousDigest, &c.CredentialDigest, &issued, &c.IssuedBy); err != nil {
			return nil, err
		}
		if c.IssuedAt, err = parseTime(issued); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (s *TxStore) LoadDatasetsForCredentials(credentials []domain.ReleaseCredential) (map[string]domain.FrozenDataset, error) {
	items := map[string]domain.FrozenDataset{}
	for _, c := range credentials {
		d, err := s.LoadDataset(c.CampaignID)
		if err != nil {
			if de, ok := err.(*domain.DomainError); ok && de.Code == domain.ErrNotFound {
				continue
			}
			return nil, err
		}
		items[c.CampaignID] = d
	}
	return items, nil
}
