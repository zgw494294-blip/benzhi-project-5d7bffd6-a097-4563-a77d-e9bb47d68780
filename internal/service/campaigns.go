package service

import (
	"context"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

const maxBatchWells = 100

func (s *Service) CreateCampaign(ctx context.Context, cmd CreateCampaignCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead); err != nil {
		return MutationResult{}, err
	}
	id := s.newID()
	now := s.now()
	return execute(ctx, s, id, cmd.IdempotencyKey, "create_campaign", func(tx *store.TxStore) (MutationResult, error) {
		c, err := domain.NewCampaign(id, cmd.CampaignCode, cmd.SamplingWindowStart, cmd.SamplingWindowEnd, cmd.RuleSetVersion, now)
		if err != nil {
			return MutationResult{}, err
		}
		if err = tx.InsertCampaign(c); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "CAMPAIGN_CREATED", cmd.Actor, map[string]any{"campaignCode": c.CampaignCode, "ruleSetVersion": c.RuleSetVersion}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: c.ID, Status: c.Status, Version: c.ExpectedVersion}, nil
	})
}

func (s *Service) AddWellsBatch(ctx context.Context, campaignID string, cmd AddWellsBatchCommand) (BatchWellResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead); err != nil {
		return BatchWellResult{}, err
	}
	if len(cmd.Items) > 0 && len(cmd.Wells) > 0 {
		return BatchWellResult{}, domain.FieldError("items", "items 与 wells 不能同时提供")
	}
	if len(cmd.Items) == 0 {
		cmd.Items = cmd.Wells
	}
	if len(cmd.Items) == 0 {
		return BatchWellResult{}, domain.FieldError("items", "批量计划不能为空")
	}
	if len(cmd.Items) > maxBatchWells {
		return BatchWellResult{}, domain.FieldError("items", "单次最多登记 100 口监测井")
	}
	ids := make([]string, len(cmd.Items))
	for i := range ids {
		ids[i] = s.newID()
	}
	now := s.now()
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, "add_wells_batch:"+campaignID, func(tx *store.TxStore) (BatchWellResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return BatchWellResult{}, err
		}
		if !c.Status.CanEditFacts() {
			return BatchWellResult{}, domain.NewError(domain.ErrState, "当前批次已锁定采样事实")
		}
		existing, err := tx.ExistingWellCodes(c.ID)
		if err != nil {
			return BatchWellResult{}, err
		}
		drafts := make([]domain.WellPlanDraft, len(cmd.Items))
		for i, item := range cmd.Items {
			drafts[i] = domain.WellPlanDraft{WellCode: item.WellCode, LocationLabel: item.LocationLabel, PlannedAnalytes: item.PlannedAnalytes, ResponsiblePerson: item.ResponsiblePerson, PlannedSampleAt: item.PlannedSampleAt}
		}
		wells, validation := domain.ValidateWellBatch(ids, c.ID, drafts, c, existing)
		if len(validation) > 0 {
			return BatchWellResult{}, &domain.BatchValidationError{Items: validation}
		}
		if err = c.BeginRecording(); err != nil {
			return BatchWellResult{}, err
		}
		for _, w := range wells {
			if err = tx.InsertWell(w); err != nil {
				return BatchWellResult{}, err
			}
		}
		c.Touch(true)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return BatchWellResult{}, err
		}
		results := make([]BatchWellItemResult, len(wells))
		for i, w := range wells {
			results[i] = BatchWellItemResult{Index: i, WellID: w.ID}
		}
		if err = appendAudit(tx, c.ID, "WELLS_BATCH_REGISTERED", cmd.Actor, map[string]any{"count": len(wells), "wellIds": ids}, now); err != nil {
			return BatchWellResult{}, err
		}
		return BatchWellResult{CampaignID: c.ID, Status: c.Status, Version: c.ExpectedVersion, FactsRevision: c.FactsRevision, Items: results}, nil
	})
}

func (s *Service) AddWell(ctx context.Context, campaignID string, cmd AddWellCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead); err != nil {
		return MutationResult{}, err
	}
	id := s.newID()
	now := s.now()
	return execute(ctx, s, campaignID, cmd.IdempotencyKey, "add_well:"+campaignID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		if !c.Status.CanEditFacts() {
			return MutationResult{}, domain.NewError(domain.ErrState, "当前批次已锁定采样事实")
		}
		w, err := domain.NewWell(id, c.ID, cmd.WellCode, cmd.LocationLabel, cmd.PlannedAnalytes, cmd.ResponsiblePerson, cmd.PlannedSampleAt, c)
		if err != nil {
			return MutationResult{}, err
		}
		if err = c.BeginRecording(); err != nil {
			return MutationResult{}, err
		}
		if err = tx.InsertWell(w); err != nil {
			return MutationResult{}, err
		}
		c.Touch(true)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "WELL_REGISTERED", cmd.Actor, map[string]any{"wellId": w.ID, "wellCode": w.WellCode}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: w.ID, Status: c.Status, Version: c.ExpectedVersion}, nil
	})
}
