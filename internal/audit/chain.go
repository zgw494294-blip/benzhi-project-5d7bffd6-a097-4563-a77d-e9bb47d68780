package audit

import (
	"encoding/json"
	"fmt"
	"time"

	"groundwater-release/internal/store"
)

type eventContent struct {
	CampaignID     string          `json:"campaignId"`
	Sequence       int64           `json:"sequence"`
	EventType      string          `json:"eventType"`
	Actor          string          `json:"actor"`
	OccurredAt     string          `json:"occurredAt"`
	Payload        json.RawMessage `json:"payload"`
	PreviousDigest string          `json:"previousDigest"`
}

func AppendEvent(existing []store.AuditEvent, campaignID, eventType, actor string, payload any, now time.Time) (store.AuditEvent, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return store.AuditEvent{}, err
	}
	sequence := int64(len(existing) + 1)
	previous := ""
	if len(existing) > 0 {
		previous = existing[len(existing)-1].Digest
	}
	now = now.UTC()
	content := eventContent{CampaignID: campaignID, Sequence: sequence, EventType: eventType, Actor: actor, OccurredAt: now.Format(time.RFC3339Nano), Payload: data, PreviousDigest: previous}
	digest, err := digestJSON(content)
	if err != nil {
		return store.AuditEvent{}, err
	}
	return store.AuditEvent{CampaignID: campaignID, Sequence: sequence, EventType: eventType, Actor: actor, OccurredAt: now, Payload: data, PreviousDigest: previous, Digest: digest}, nil
}

func VerifyChain(events []store.AuditEvent) error {
	if failure, err := VerifyChainDetailed(events); err != nil {
		return fmt.Errorf("%s", failure.Message)
	}
	return nil
}

type ChainFailure struct {
	Sequence int64
	Code     string
	Expected string
	Actual   string
	Message  string
}

func VerifyChainDetailed(events []store.AuditEvent) (*ChainFailure, error) {
	previous := ""
	for i, e := range events {
		expectedSequence := int64(i + 1)
		if e.Sequence != expectedSequence {
			return &ChainFailure{Sequence: e.Sequence, Code: "AUDIT_SEQUENCE_GAP", Expected: fmt.Sprint(expectedSequence), Actual: fmt.Sprint(e.Sequence), Message: fmt.Sprintf("审计事件序号不连续: 期望 %d", expectedSequence)}, fmt.Errorf("审计事件序号不连续")
		}
		if e.PreviousDigest != previous {
			return &ChainFailure{Sequence: e.Sequence, Code: "AUDIT_PREVIOUS_DIGEST_MISMATCH", Expected: previous, Actual: e.PreviousDigest, Message: fmt.Sprintf("审计事件 %d 的前序摘要不匹配", e.Sequence)}, fmt.Errorf("审计事件前序摘要不匹配")
		}
		content := eventContent{CampaignID: e.CampaignID, Sequence: e.Sequence, EventType: e.EventType, Actor: e.Actor, OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339Nano), Payload: e.Payload, PreviousDigest: e.PreviousDigest}
		digest, err := digestJSON(content)
		if err != nil {
			return &ChainFailure{Sequence: e.Sequence, Code: "AUDIT_DIGEST_ERROR", Message: err.Error()}, err
		}
		if digest != e.Digest {
			return &ChainFailure{Sequence: e.Sequence, Code: "AUDIT_DIGEST_MISMATCH", Expected: digest, Actual: e.Digest, Message: fmt.Sprintf("审计事件 %d 的摘要不匹配", e.Sequence)}, fmt.Errorf("审计事件摘要不匹配")
		}
		previous = e.Digest
	}
	return nil, nil
}
