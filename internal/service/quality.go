package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

type CheckResult struct {
	MutationResult
	Check      domain.QualityCheck       `json:"check"`
	Exceptions []domain.QualityException `json:"exceptions"`
}

func (s *Service) RunQualityCheck(ctx context.Context, campaignID string, cmd RunCheckCommand) (CheckResult, error) {
	if err := authorize(cmd.CommandMeta, RoleQualityReviewer); err != nil {
		return CheckResult{}, err
	}
	checkID, now := s.newID(), s.now()
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, "quality_check:"+campaignID, func(tx *store.TxStore) (CheckResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return CheckResult{}, err
		}
		wells, err := tx.ListWells(c.ID)
		if err != nil {
			return CheckResult{}, err
		}
		samples, err := tx.ListSamples(c.ID)
		if err != nil {
			return CheckResult{}, err
		}
		check, err := domain.EvaluateQuality(c, wells, samples, checkID, cmd.Actor, now)
		if err != nil {
			return CheckResult{}, err
		}
		if digest, found, findErr := tx.FindCheckDigest(c.ID, c.RuleSetVersion, c.FactsRevision); findErr != nil {
			return CheckResult{}, findErr
		} else if found && digest != check.ResultDigest {
			return CheckResult{}, domain.NewError(domain.ErrIntegrity, "相同规则版本与事实修订号产生了不一致的检查摘要")
		}
		if err = c.MarkChecked(); err != nil {
			return CheckResult{}, err
		}
		check.Current = true
		if err = tx.InsertQualityCheck(check); err != nil {
			return CheckResult{}, err
		}
		exceptions := []domain.QualityException{}
		for _, result := range check.Results {
			if result.Passed {
				continue
			}
			e := domain.QualityException{ID: s.newID(), CampaignID: c.ID, CheckID: check.ID, RuleCode: result.RuleCode, SubjectID: result.SubjectID, Severity: result.Severity, Description: result.Message, Status: domain.ExceptionOpen, EvidenceRevisions: []domain.EvidenceRevision{}, Current: true}
			if err = tx.InsertException(e); err != nil {
				return CheckResult{}, err
			}
			exceptions = append(exceptions, e)
		}
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return CheckResult{}, err
		}
		if err = appendAudit(tx, c.ID, "QUALITY_CHECKED", cmd.Actor, map[string]any{"checkId": check.ID, "factsRevision": check.FactsRevision, "resultDigest": check.ResultDigest, "failedCount": len(exceptions)}, now); err != nil {
			return CheckResult{}, err
		}
		return CheckResult{MutationResult: MutationResult{CampaignID: c.ID, ResourceID: check.ID, Status: c.Status, Version: c.ExpectedVersion, CheckID: check.ID, CheckDigest: check.ResultDigest}, Check: check, Exceptions: exceptions}, nil
	})
}

func (s *Service) ReopenCheck(ctx context.Context, campaignID string, cmd ReopenCheckCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleQualityReviewer); err != nil {
		return MutationResult{}, err
	}
	reason := strings.TrimSpace(cmd.Reason)
	if reason == "" {
		return MutationResult{}, domain.FieldError("reason", "退回原因不能为空")
	}
	now := s.now()
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, "reopen_check:"+campaignID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		check, err := ensureCheckFresh(tx, c)
		if err != nil {
			return MutationResult{}, err
		}
		if err = c.ReopenForRevision(); err != nil {
			return MutationResult{}, err
		}
		if err = tx.InvalidateCurrentCheck(c.ID, reason, now); err != nil {
			return MutationResult{}, err
		}
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "QUALITY_CHECK_REOPENED", cmd.Actor, map[string]any{"checkId": check.ID, "reason": reason}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: check.ID, Status: c.Status, Version: c.ExpectedVersion}, nil
	})
}

func (s *Service) AddEvidence(ctx context.Context, campaignID, exceptionID string, cmd AddEvidenceCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead, RoleLabReceiver); err != nil {
		return MutationResult{}, err
	}
	now := s.now()
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, fmt.Sprintf("add_evidence:%s:%s", campaignID, exceptionID), func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		if c.Status != domain.StatusChecked {
			return MutationResult{}, domain.NewError(domain.ErrState, "仅已检查批次可以提交整改证据")
		}
		if _, err = ensureCheckFresh(tx, c); err != nil {
			return MutationResult{}, err
		}
		e, err := tx.LoadException(c.ID, exceptionID)
		if err != nil {
			return MutationResult{}, err
		}
		revision, err := e.AddEvidence(cmd.Kind, cmd.Content, cmd.Actor, cmd.ReferencesRevision, now)
		if err != nil {
			return MutationResult{}, err
		}
		if err = tx.UpdateException(e); err != nil {
			return MutationResult{}, err
		}
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		summary := e.EvidenceRevisions[len(e.EvidenceRevisions)-1].ContentSummary
		if err = appendAudit(tx, c.ID, "EVIDENCE_SUBMITTED", cmd.Actor, map[string]any{"exceptionId": e.ID, "revision": revision, "kind": cmd.Kind, "contentSummary": summary}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: e.ID, Status: c.Status, Version: c.ExpectedVersion, EvidenceRevision: revision}, nil
	})
}

func (s *Service) WithdrawEvidence(ctx context.Context, campaignID, exceptionID string, revision int64, cmd WithdrawEvidenceCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead, RoleLabReceiver); err != nil {
		return MutationResult{}, err
	}
	if cmd.ExpectedEvidenceRevision != revision {
		return MutationResult{}, &domain.DomainError{Code: domain.ErrStaleVersion, Field: "expectedEvidenceRevision", Message: "路径 revision 与 expectedEvidenceRevision 不一致"}
	}
	now := s.now()
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, fmt.Sprintf("withdraw_evidence:%s:%s:%d", campaignID, exceptionID, revision), func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		if c.Status != domain.StatusChecked {
			return MutationResult{}, domain.NewError(domain.ErrState, "仅已检查批次可以撤回整改证据")
		}
		if _, err = ensureCheckFresh(tx, c); err != nil {
			return MutationResult{}, err
		}
		e, err := tx.LoadException(c.ID, exceptionID)
		if err != nil {
			return MutationResult{}, err
		}
		if err = e.Withdraw(revision, cmd.Actor, now); err != nil {
			return MutationResult{}, err
		}
		if err = tx.UpdateException(e); err != nil {
			return MutationResult{}, err
		}
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "EVIDENCE_WITHDRAWN", cmd.Actor, map[string]any{"exceptionId": e.ID, "revision": revision, "contentSummary": e.EvidenceRevisions[len(e.EvidenceRevisions)-1].ContentSummary}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: e.ID, Status: c.Status, Version: c.ExpectedVersion, EvidenceRevision: revision}, nil
	})
}

func (s *Service) ReviewException(ctx context.Context, campaignID, exceptionID string, cmd ReviewExceptionCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleQualityReviewer); err != nil {
		return MutationResult{}, err
	}
	now := s.now()
	if cmd.EvidenceRevision > 0 && cmd.Revision > 0 && cmd.EvidenceRevision != cmd.Revision {
		return MutationResult{}, domain.FieldError("evidenceRevision", "evidenceRevision 与 revision 不一致")
	}
	if cmd.EvidenceRevision == 0 {
		cmd.EvidenceRevision = cmd.Revision
	}
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, fmt.Sprintf("review_exception:%s:%s", campaignID, exceptionID), func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		if c.Status != domain.StatusChecked {
			return MutationResult{}, domain.NewError(domain.ErrState, "仅已检查批次可以审核整改")
		}
		if _, err = ensureCheckFresh(tx, c); err != nil {
			return MutationResult{}, err
		}
		e, err := tx.LoadException(c.ID, exceptionID)
		if err != nil {
			return MutationResult{}, err
		}
		if err = e.Review(cmd.EvidenceRevision, cmd.Decision, cmd.Comment, cmd.Actor, now); err != nil {
			return MutationResult{}, err
		}
		if err = tx.UpdateException(e); err != nil {
			return MutationResult{}, err
		}
		c.Touch(false)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		latest := e.EvidenceRevisions[len(e.EvidenceRevisions)-1]
		if err = appendAudit(tx, c.ID, "EVIDENCE_REVIEWED", cmd.Actor, map[string]any{"exceptionId": e.ID, "revision": latest.Revision, "decision": cmd.Decision, "contentSummary": latest.ContentSummary}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: e.ID, Status: c.Status, Version: c.ExpectedVersion, EvidenceRevision: latest.Revision}, nil
	})
}

type FailureRef struct {
	RuleCode  string `json:"ruleCode"`
	SubjectID string `json:"subjectId,omitempty"`
}
type CheckDifference struct {
	FromCheckID        string       `json:"fromCheckId"`
	ToCheckID          string       `json:"toCheckId"`
	NewFailures        []FailureRef `json:"newFailures"`
	ResolvedFailures   []FailureRef `json:"resolvedFailures"`
	Passed             []FailureRef `json:"passed"`
	PersistentFailures []FailureRef `json:"persistentFailures"`
}
type CheckHistoryResult struct {
	Items      []domain.QualityCheck `json:"items"`
	Limit      int                   `json:"limit"`
	Offset     int                   `json:"offset"`
	Total      int                   `json:"total"`
	Difference *CheckDifference      `json:"difference,omitempty"`
}

func failedRefs(q domain.QualityCheck) map[string]FailureRef {
	m := map[string]FailureRef{}
	for _, r := range q.Results {
		if !r.Passed {
			m[domain.FailedKey(r)] = FailureRef{RuleCode: r.RuleCode, SubjectID: r.SubjectID}
		}
	}
	return m
}
func (s *Service) CheckHistory(ctx context.Context, campaignID string, limit, offset int, fromID, toID string) (CheckHistoryResult, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	result := CheckHistoryResult{Limit: limit, Offset: offset}
	err := s.repo.View(ctx, func(tx *store.TxStore) error {
		var err error
		if _, err = tx.LoadCampaign(campaignID); err != nil {
			return err
		}
		if result.Items, err = tx.ListChecks(campaignID, limit, offset); err != nil {
			return err
		}
		if result.Total, err = tx.CountChecks(campaignID); err != nil {
			return err
		}
		if fromID == "" && toID == "" {
			return nil
		}
		if fromID == "" || toID == "" {
			return domain.FieldError("fromCheckId", "差异查询必须同时提供 fromCheckId 和 toCheckId")
		}
		from, err := tx.LoadCheck(campaignID, fromID)
		if err != nil {
			return err
		}
		to, err := tx.LoadCheck(campaignID, toID)
		if err != nil {
			return err
		}
		a, b := failedRefs(from), failedRefs(to)
		d := &CheckDifference{FromCheckID: fromID, ToCheckID: toID, NewFailures: []FailureRef{}, ResolvedFailures: []FailureRef{}, PersistentFailures: []FailureRef{}}
		for k, v := range b {
			if _, ok := a[k]; ok {
				d.PersistentFailures = append(d.PersistentFailures, v)
			} else {
				d.NewFailures = append(d.NewFailures, v)
			}
		}
		for k, v := range a {
			if _, ok := b[k]; !ok {
				d.ResolvedFailures = append(d.ResolvedFailures, v)
			}
		}
		result.Difference = d
		sort.Slice(d.NewFailures, func(i, j int) bool {
			return d.NewFailures[i].RuleCode+":"+d.NewFailures[i].SubjectID < d.NewFailures[j].RuleCode+":"+d.NewFailures[j].SubjectID
		})
		sort.Slice(d.ResolvedFailures, func(i, j int) bool {
			return d.ResolvedFailures[i].RuleCode+":"+d.ResolvedFailures[i].SubjectID < d.ResolvedFailures[j].RuleCode+":"+d.ResolvedFailures[j].SubjectID
		})
		sort.Slice(d.PersistentFailures, func(i, j int) bool {
			return d.PersistentFailures[i].RuleCode+":"+d.PersistentFailures[i].SubjectID < d.PersistentFailures[j].RuleCode+":"+d.PersistentFailures[j].SubjectID
		})
		d.Passed = append([]FailureRef(nil), d.ResolvedFailures...)
		return nil
	})
	return result, err
}
