package service

import (
	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

func statusToDomain(v string) domain.CampaignStatus { return domain.CampaignStatus(v) }

func loadForWrite(tx *store.TxStore, campaignID string, meta CommandMeta) (domain.MonitoringCampaign, int64, error) {
	c, err := tx.LoadCampaign(campaignID)
	if err != nil {
		return c, 0, err
	}
	if err = c.EnsureVersion(meta.ExpectedVersion); err != nil {
		return c, 0, err
	}
	return c, c.ExpectedVersion, nil
}

func ensureCheckFresh(tx *store.TxStore, c domain.MonitoringCampaign) (domain.QualityCheck, error) {
	q, err := tx.LoadQualityCheck(c.ID)
	if err != nil {
		if de, ok := err.(*domain.DomainError); ok && de.Code == domain.ErrNotFound {
			return q, domain.NewError(domain.ErrConflict, "当前没有有效质量检查，请重新检查")
		}
		return q, err
	}
	if !q.Fresh(c) {
		return q, domain.NewError(domain.ErrConflict, "质量检查已因采样事实变化而过期，请重新检查")
	}
	return q, nil
}
