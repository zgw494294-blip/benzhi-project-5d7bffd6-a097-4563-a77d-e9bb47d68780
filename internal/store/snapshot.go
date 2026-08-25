package store

import (
	"context"

	"groundwater-release/internal/domain"
)

type CampaignSnapshot struct {
	Campaign              domain.MonitoringCampaign                  `json:"campaign"`
	Wells                 []domain.MonitoringWell                    `json:"wells"`
	Samples               []domain.SampleRecord                      `json:"samples"`
	QualityCheck          *domain.QualityCheck                       `json:"qualityCheck,omitempty"`
	CheckHistory          []domain.QualityCheck                      `json:"checkHistory"`
	Exceptions            []domain.QualityException                  `json:"exceptions"`
	SampleRevisionHistory map[string][]domain.SampleRevisionSnapshot `json:"sampleRevisionHistory"`
	Dataset               *domain.FrozenDataset                      `json:"dataset,omitempty"`
	Credential            *domain.ReleaseCredential                  `json:"credential,omitempty"`
	Timeline              []AuditEvent                               `json:"timeline"`
}

func (r *Repository) Snapshot(ctx context.Context, campaignID string) (CampaignSnapshot, error) {
	var result CampaignSnapshot
	err := r.View(ctx, func(s *TxStore) error {
		var err error
		if result.Campaign, err = s.LoadCampaign(campaignID); err != nil {
			return err
		}
		if result.Wells, err = s.ListWells(campaignID); err != nil {
			return err
		}
		if result.Samples, err = s.ListSamples(campaignID); err != nil {
			return err
		}
		if result.CheckHistory, err = s.ListChecks(campaignID, 100, 0); err != nil {
			return err
		}
		result.SampleRevisionHistory = map[string][]domain.SampleRevisionSnapshot{}
		for _, sample := range result.Samples {
			history, historyErr := s.ListSampleHistory(sample.ID)
			if historyErr != nil {
				return historyErr
			}
			if len(history) > 0 {
				result.SampleRevisionHistory[sample.ID] = history
			}
		}
		q, qerr := s.LoadQualityCheck(campaignID)
		if qerr == nil {
			result.QualityCheck = &q
		} else if de, ok := qerr.(*domain.DomainError); !ok || de.Code != domain.ErrNotFound {
			return qerr
		}
		if result.Exceptions, err = s.ListExceptions(campaignID); err != nil {
			return err
		}
		d, derr := s.LoadDataset(campaignID)
		if derr == nil {
			result.Dataset = &d
		} else if de, ok := derr.(*domain.DomainError); !ok || de.Code != domain.ErrNotFound {
			return derr
		}
		if result.Credential, err = s.LoadCredential(campaignID); err != nil {
			return err
		}
		result.Timeline, err = s.ListAuditEvents(campaignID)
		return err
	})
	return result, err
}
