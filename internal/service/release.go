package service

import (
	"context"
	"strings"

	"groundwater-release/internal/audit"
	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

type ApprovalBlocker struct {
	Code     string `json:"code"`
	ObjectID string `json:"objectId,omitempty"`
	Message  string `json:"message"`
}

type ApprovalReadiness struct {
	CampaignID               string            `json:"campaignId"`
	Version                  int64             `json:"version"`
	Ready                    bool              `json:"ready"`
	CheckID                  string            `json:"checkId,omitempty"`
	CheckDigest              string            `json:"checkDigest,omitempty"`
	UnclosedExceptionCount   int               `json:"unclosedExceptionCount"`
	PendingEvidenceCount     int               `json:"pendingEvidenceCount"`
	UnresolvedRuleErrorCount int               `json:"unresolvedRuleErrorCount"`
	Blockers                 []ApprovalBlocker `json:"blockers"`
}

func assessApproval(c domain.MonitoringCampaign, check *domain.QualityCheck, exceptions []domain.QualityException, actor string) ApprovalReadiness {
	r := ApprovalReadiness{CampaignID: c.ID, Version: c.ExpectedVersion, Blockers: []ApprovalBlocker{}}
	if actor == "" {
		r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "APPROVER_IDENTITY_REQUIRED", ObjectID: c.ID, Message: "就绪评估需要提供技术批准人身份"})
	}
	if c.Status != domain.StatusChecked {
		r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "CAMPAIGN_STATE", ObjectID: c.ID, Message: "批次不是已检查状态"})
	}
	if check == nil {
		r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "CURRENT_CHECK_MISSING", ObjectID: c.ID, Message: "不存在当前有效质量检查"})
		return r
	}
	r.CheckID, r.CheckDigest = check.ID, check.ResultDigest
	if !check.Fresh(c) {
		r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "CHECK_STALE", ObjectID: check.ID, Message: "当前质量检查已过期"})
	}
	if actor != "" && actor == check.CheckedBy {
		r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "CHECKER_APPROVER_CONFLICT", ObjectID: check.ID, Message: "技术批准人不得与当前质量检查执行人相同"})
	}
	for _, e := range exceptions {
		if e.Status != domain.ExceptionClosed {
			r.UnclosedExceptionCount++
			if e.Severity == domain.SeverityError {
				r.UnresolvedRuleErrorCount++
			}
			r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "EXCEPTION_OPEN", ObjectID: e.ID, Message: "质量异常尚未关闭"})
		}
		if len(e.EvidenceRevisions) > 0 {
			latest := e.EvidenceRevisions[len(e.EvidenceRevisions)-1]
			if latest.Status == domain.EvidencePending {
				r.PendingEvidenceCount++
				r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "EVIDENCE_PENDING", ObjectID: e.ID, Message: "整改证据仍待审核"})
			}
			if actor != "" && latest.Status == domain.EvidenceAccepted && latest.ReviewedBy == actor {
				r.Blockers = append(r.Blockers, ApprovalBlocker{Code: "EVIDENCE_REVIEWER_APPROVER_CONFLICT", ObjectID: e.ID, Message: "技术批准人不得批准自己接受的整改证据"})
			}
		}
	}
	r.Ready = len(r.Blockers) == 0
	return r
}

func (s *Service) ApprovalReadiness(ctx context.Context, campaignID, actor string) (ApprovalReadiness, error) {
	var result ApprovalReadiness
	err := s.repo.View(ctx, func(tx *store.TxStore) error {
		c, err := tx.LoadCampaign(campaignID)
		if err != nil {
			return err
		}
		var check *domain.QualityCheck
		q, qerr := tx.LoadQualityCheck(c.ID)
		if qerr == nil {
			check = &q
		} else if de, ok := qerr.(*domain.DomainError); !ok || de.Code != domain.ErrNotFound {
			return qerr
		}
		exceptions, err := tx.ListCurrentExceptions(c.ID)
		if err != nil {
			return err
		}
		result = assessApproval(c, check, exceptions, strings.TrimSpace(actor))
		return nil
	})
	return result, err
}

func (s *Service) Approve(ctx context.Context, campaignID string, cmd ApproveCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleTechnicalApprover); err != nil {
		return MutationResult{}, err
	}
	now := s.now()
	return execute(ctx, s, cmd.IdempotencyKey, "approve:"+campaignID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		check, err := ensureCheckFresh(tx, c)
		if err != nil {
			return MutationResult{}, err
		}
		if strings.TrimSpace(cmd.CheckDigest) == "" {
			return MutationResult{}, domain.FieldError("checkDigest", "批准必须携带已核对的 checkDigest")
		}
		if cmd.CheckDigest != check.ResultDigest {
			return MutationResult{}, &domain.DomainError{Code: domain.ErrStaleVersion, Field: "checkDigest", Message: "checkDigest 与当前有效检查不一致"}
		}
		exceptions, err := tx.ListCurrentExceptions(c.ID)
		if err != nil {
			return MutationResult{}, err
		}
		readiness := assessApproval(c, &check, exceptions, cmd.Actor)
		if !readiness.Ready {
			return MutationResult{}, domain.NewError(domain.ErrConflict, readiness.Blockers[0].Message)
		}
		if err = c.Approve(now); err != nil {
			return MutationResult{}, err
		}
		c.ApprovedCheckID, c.ApprovedCheckDigest = check.ID, check.ResultDigest
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "TECHNICALLY_APPROVED", cmd.Actor, map[string]any{"approvedAt": now.UTC(), "checkId": check.ID, "checkDigest": check.ResultDigest}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, Status: c.Status, Version: c.ExpectedVersion, CheckID: check.ID, CheckDigest: check.ResultDigest}, nil
	})
}

func (s *Service) Freeze(ctx context.Context, campaignID string, cmd FreezeCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleTechnicalApprover); err != nil {
		return MutationResult{}, err
	}
	now := s.now()
	return execute(ctx, s, cmd.IdempotencyKey, "freeze:"+campaignID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		check, err := ensureCheckFresh(tx, c)
		if err != nil {
			return MutationResult{}, err
		}
		wells, err := tx.ListWells(c.ID)
		if err != nil {
			return MutationResult{}, err
		}
		samples, err := tx.ListSamples(c.ID)
		if err != nil {
			return MutationResult{}, err
		}
		exceptions, err := tx.ListCurrentExceptions(c.ID)
		if err != nil {
			return MutationResult{}, err
		}
		if err = c.Freeze(); err != nil {
			return MutationResult{}, err
		}
		c.Touch(false)
		dataset, err := domain.BuildFrozenDataset(c, wells, samples, check, exceptions, 1, cmd.Actor, now)
		if err != nil {
			return MutationResult{}, err
		}
		if err = tx.InsertDataset(dataset); err != nil {
			return MutationResult{}, err
		}
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "DATASET_FROZEN", cmd.Actor, map[string]any{"datasetVersion": dataset.DatasetVersion, "datasetDigest": dataset.Digest}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: dataset.Digest, Status: c.Status, Version: c.ExpectedVersion}, nil
	})
}

func (s *Service) IssueCredential(ctx context.Context, campaignID string, cmd IssueCredentialCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleReleaseOfficer); err != nil {
		return MutationResult{}, err
	}
	now := s.now()
	id := s.newID()
	return execute(ctx, s, cmd.IdempotencyKey, "issue_credential:"+campaignID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		dataset, err := tx.LoadDataset(c.ID)
		if err != nil {
			return MutationResult{}, err
		}
		serial, previousDigest, err := tx.NextCredentialSerial()
		if err != nil {
			return MutationResult{}, err
		}
		credential, err := audit.IssueCredential(id, dataset, serial, previousDigest, cmd.Actor, now)
		if err != nil {
			return MutationResult{}, err
		}
		if err = tx.InsertCredential(credential); err != nil {
			return MutationResult{}, err
		}
		if err = c.Release(); err != nil {
			return MutationResult{}, err
		}
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "CREDENTIAL_ISSUED", cmd.Actor, map[string]any{"credentialId": credential.ID, "serialNumber": credential.SerialNumber, "credentialDigest": credential.CredentialDigest}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: credential.ID, Status: c.Status, Version: c.ExpectedVersion}, nil
	})
}
