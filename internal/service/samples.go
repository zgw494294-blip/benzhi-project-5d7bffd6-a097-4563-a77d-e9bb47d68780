package service

import (
	"context"
	"strings"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

func (s *Service) AddSample(ctx context.Context, campaignID string, cmd AddSampleCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead, RoleLabReceiver); err != nil {
		return MutationResult{}, err
	}
	id := s.newID()
	now := s.now()
	return execute(ctx, s, cmd.IdempotencyKey, "add_sample:"+campaignID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		if !c.Status.CanEditFacts() {
			return MutationResult{}, domain.NewError(domain.ErrState, "质量检查后禁止继续修改采样事实")
		}
		wellExists := false
		if cmd.WellID != "" {
			wellExists, err = tx.WellExists(c.ID, cmd.WellID)
			if err != nil {
				return MutationResult{}, err
			}
		}
		sample := domain.SampleRecord{ID: id, CampaignID: c.ID, WellID: cmd.WellID, SampleCode: cmd.SampleCode, SampleKind: cmd.SampleKind, CollectedAt: cmd.CollectedAt.UTC(), FieldMeasurements: cmd.FieldMeasurements, PreservationMethod: cmd.PreservationMethod, PreservationExpiresAt: cmd.PreservationExpiresAt.UTC(), CustodyEvents: cmd.CustodyEvents, Revision: 1}
		validationKey := strings.TrimSpace(sample.SampleCode)
		if !s.sampleValidationKnown(validationKey) {
			if err = sample.Validate(c, wellExists); err != nil {
				return MutationResult{}, err
			}
			s.rememberSampleValidation(validationKey)
		}
		if err = c.BeginRecording(); err != nil {
			return MutationResult{}, err
		}
		if err = tx.InsertSample(sample); err != nil {
			return MutationResult{}, err
		}
		c.Touch(true)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "SAMPLE_RECORDED", cmd.Actor, map[string]any{"sampleId": sample.ID, "sampleCode": sample.SampleCode, "sampleKind": sample.SampleKind}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: sample.ID, Status: c.Status, Version: c.ExpectedVersion, FactsRevision: c.FactsRevision, SampleRevision: sample.Revision}, nil
	})
}

func (s *Service) ReviseSample(ctx context.Context, campaignID, sampleID string, cmd ReviseSampleCommand) (MutationResult, error) {
	if err := authorize(cmd.CommandMeta, RoleFieldLead, RoleLabReceiver); err != nil {
		return MutationResult{}, err
	}
	if cmd.Revision > 0 && cmd.ExpectedRevision > 0 && cmd.Revision != cmd.ExpectedRevision {
		return MutationResult{}, domain.FieldError("revision", "revision 与 expectedRevision 不一致")
	}
	if cmd.Revision == 0 {
		cmd.Revision = cmd.ExpectedRevision
	}
	if len(cmd.AppendCustodyEvents) > 0 && len(cmd.CustodyEvents) > 0 {
		return MutationResult{}, domain.FieldError("appendCustodyEvents", "两种交接追加字段不能同时提供")
	}
	if len(cmd.AppendCustodyEvents) == 0 {
		cmd.AppendCustodyEvents = cmd.CustodyEvents
	}
	if cmd.FieldMeasurements == nil && cmd.PreservationMethod == nil && cmd.PreservationExpiresAt == nil && len(cmd.AppendCustodyEvents) == 0 {
		return MutationResult{}, domain.FieldError("body", "至少需要修改一项样品事实")
	}
	now := s.now()
	return execute(ctx, s, cmd.IdempotencyKey, "revise_sample:"+campaignID+":"+sampleID, func(tx *store.TxStore) (MutationResult, error) {
		c, previous, err := loadForWrite(tx, campaignID, cmd.CommandMeta)
		if err != nil {
			return MutationResult{}, err
		}
		if c.Status != domain.StatusRecording {
			return MutationResult{}, domain.NewError(domain.ErrState, "样品仅能在批次退回记录中后修订")
		}
		before, err := tx.LoadSample(c.ID, sampleID)
		if err != nil {
			return MutationResult{}, err
		}
		wellExists := false
		if before.WellID != "" {
			wellExists, err = tx.WellExists(c.ID, before.WellID)
			if err != nil {
				return MutationResult{}, err
			}
		}
		after, err := before.Revise(cmd.FieldMeasurements, cmd.PreservationMethod, cmd.PreservationExpiresAt, cmd.AppendCustodyEvents, cmd.Revision, c, wellExists)
		if err != nil {
			return MutationResult{}, err
		}
		if err = tx.ReviseSample(before, after, cmd.Actor, now); err != nil {
			return MutationResult{}, err
		}
		c.Touch(true)
		if err = tx.SaveCampaign(c, previous); err != nil {
			return MutationResult{}, err
		}
		if err = appendAudit(tx, c.ID, "SAMPLE_REVISED", cmd.Actor, map[string]any{"sampleId": sampleID, "fromRevision": before.Revision, "toRevision": after.Revision, "measurementCount": len(after.FieldMeasurements), "custodyCount": len(after.CustodyEvents)}, now); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{CampaignID: c.ID, ResourceID: sampleID, Status: c.Status, Version: c.ExpectedVersion, FactsRevision: c.FactsRevision, SampleRevision: after.Revision}, nil
	})
}
