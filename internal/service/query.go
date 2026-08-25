package service

import (
	"context"
	"encoding/json"

	"groundwater-release/internal/audit"
	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

type CampaignDetail struct {
	store.CampaignSnapshot
	PendingExceptions int                `json:"pendingExceptions"`
	Verification      audit.Verification `json:"verification"`
}

func (s *Service) CampaignDetail(ctx context.Context, campaignID string) (CampaignDetail, error) {
	if err := ctx.Err(); err != nil {
		return CampaignDetail{}, err
	}
	if cached, ok := s.cachedDetail(campaignID); ok {
		var detail CampaignDetail
		if err := json.Unmarshal(cached, &detail); err != nil {
			return CampaignDetail{}, err
		}
		return detail, nil
	}
	snapshot, err := s.repo.Snapshot(ctx, campaignID)
	if err != nil {
		return CampaignDetail{}, err
	}
	pending := 0
	for _, e := range snapshot.Exceptions {
		if e.Current && e.Status != domain.ExceptionClosed {
			pending++
		}
	}
	var previous *domain.ReleaseCredential
	if snapshot.Credential != nil && snapshot.Credential.SerialNumber > 1 {
		err = s.repo.View(ctx, func(tx *store.TxStore) error {
			var inner error
			previous, inner = tx.PreviousCredential(snapshot.Credential.SerialNumber)
			return inner
		})
		if err != nil {
			return CampaignDetail{}, err
		}
	}
	verification := audit.Verify(snapshot, previous)
	if snapshot.Credential != nil {
		full, fullErr := s.CredentialVerification(ctx, campaignID)
		if fullErr != nil {
			return CampaignDetail{}, fullErr
		}
		verification.AuditChainValid = full.Items.AuditChain
		verification.DatasetDigestValid = full.Items.DatasetDigests
		verification.CredentialValid = full.Items.SequenceChain && full.Items.CredentialDigests
		if !full.Valid && full.FirstFailure != nil {
			verification.Message = full.FirstFailure.Message
		} else {
			verification.Message = "完整性校验通过"
		}
	}
	detail := CampaignDetail{CampaignSnapshot: snapshot, PendingExceptions: pending, Verification: verification}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return CampaignDetail{}, err
	}
	s.rememberDetail(campaignID, encoded)
	return detail, nil
}

func (s *Service) CredentialVerification(ctx context.Context, campaignID string) (audit.FullChainVerification, error) {
	var report audit.FullChainVerification
	err := s.repo.View(ctx, func(tx *store.TxStore) error {
		if _, err := tx.LoadCampaign(campaignID); err != nil {
			return err
		}
		target, err := tx.LoadCredential(campaignID)
		if err != nil {
			return err
		}
		if target == nil {
			return domain.NewError(domain.ErrNotFound, "该批次尚未签发放行凭据")
		}
		credentials, err := tx.ListCredentialsThrough(target.SerialNumber)
		if err != nil {
			return err
		}
		datasets, err := tx.LoadDatasetsForCredentials(credentials)
		if err != nil {
			return err
		}
		events, err := tx.ListAuditEvents(campaignID)
		if err != nil {
			return err
		}
		report = audit.VerifyCredentialChain(*target, credentials, datasets, events)
		return nil
	})
	return report, err
}
