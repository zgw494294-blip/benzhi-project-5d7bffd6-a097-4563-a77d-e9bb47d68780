package service

import (
	"time"

	"groundwater-release/internal/domain"
)

type CommandMeta struct {
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Actor           string `json:"actor"`
	Role            string `json:"role"`
}

type CreateCampaignCommand struct {
	CommandMeta
	CampaignCode        string    `json:"campaignCode"`
	SamplingWindowStart time.Time `json:"samplingWindowStart"`
	SamplingWindowEnd   time.Time `json:"samplingWindowEnd"`
	RuleSetVersion      string    `json:"ruleSetVersion"`
}

type AddWellCommand struct {
	CommandMeta
	WellCode          string    `json:"wellCode"`
	LocationLabel     string    `json:"locationLabel"`
	PlannedAnalytes   []string  `json:"plannedAnalytes"`
	ResponsiblePerson string    `json:"responsiblePerson"`
	PlannedSampleAt   time.Time `json:"plannedSampleAt"`
}

type WellPlan struct {
	WellCode          string    `json:"wellCode"`
	LocationLabel     string    `json:"locationLabel"`
	PlannedAnalytes   []string  `json:"plannedAnalytes"`
	ResponsiblePerson string    `json:"responsiblePerson"`
	PlannedSampleAt   time.Time `json:"plannedSampleAt"`
}

type AddWellsBatchCommand struct {
	CommandMeta
	Items []WellPlan `json:"items"`
	Wells []WellPlan `json:"wells,omitempty"`
}

type AddSampleCommand struct {
	CommandMeta
	WellID                string                    `json:"wellId"`
	SampleCode            string                    `json:"sampleCode"`
	SampleKind            domain.SampleKind         `json:"sampleKind"`
	CollectedAt           time.Time                 `json:"collectedAt"`
	FieldMeasurements     []domain.FieldMeasurement `json:"fieldMeasurements"`
	PreservationMethod    string                    `json:"preservationMethod"`
	PreservationExpiresAt time.Time                 `json:"preservationExpiresAt"`
	CustodyEvents         []domain.CustodyEvent     `json:"custodyEvents"`
}

type RunCheckCommand struct{ CommandMeta }

type ReopenCheckCommand struct {
	CommandMeta
	Reason string `json:"reason"`
}

type ReviseSampleCommand struct {
	CommandMeta
	Revision              int64                      `json:"revision"`
	ExpectedRevision      int64                      `json:"expectedRevision,omitempty"`
	FieldMeasurements     *[]domain.FieldMeasurement `json:"fieldMeasurements,omitempty"`
	PreservationMethod    *string                    `json:"preservationMethod,omitempty"`
	PreservationExpiresAt *time.Time                 `json:"preservationExpiresAt,omitempty"`
	AppendCustodyEvents   []domain.CustodyEvent      `json:"appendCustodyEvents,omitempty"`
	CustodyEvents         []domain.CustodyEvent      `json:"custodyEvents,omitempty"`
}

type AddEvidenceCommand struct {
	CommandMeta
	Kind               domain.EvidenceKind `json:"kind"`
	Content            string              `json:"content"`
	ReferencesRevision int64               `json:"referencesRevision,omitempty"`
}

type ReviewExceptionCommand struct {
	CommandMeta
	Decision         domain.ReviewDecision `json:"decision"`
	Comment          string                `json:"comment"`
	EvidenceRevision int64                 `json:"evidenceRevision"`
	Revision         int64                 `json:"revision,omitempty"`
}

type WithdrawEvidenceCommand struct {
	CommandMeta
	ExpectedEvidenceRevision int64 `json:"expectedEvidenceRevision"`
}

type ApproveCommand struct {
	CommandMeta
	CheckDigest string `json:"checkDigest"`
}

type FreezeCommand struct{ CommandMeta }

type IssueCredentialCommand struct{ CommandMeta }

type MutationResult struct {
	CampaignID       string                `json:"campaignId"`
	ResourceID       string                `json:"resourceId,omitempty"`
	Status           domain.CampaignStatus `json:"status"`
	Version          int64                 `json:"version"`
	Replayed         bool                  `json:"replayed,omitempty"`
	EvidenceRevision int64                 `json:"evidenceRevision,omitempty"`
	CheckID          string                `json:"checkId,omitempty"`
	CheckDigest      string                `json:"checkDigest,omitempty"`
	FactsRevision    int64                 `json:"factsRevision,omitempty"`
	SampleRevision   int64                 `json:"sampleRevision,omitempty"`
}

type BatchWellItemResult struct {
	Index  int    `json:"index"`
	WellID string `json:"wellId"`
}
type BatchWellResult struct {
	CampaignID    string                `json:"campaignId"`
	Status        domain.CampaignStatus `json:"status"`
	Version       int64                 `json:"version"`
	FactsRevision int64                 `json:"factsRevision"`
	Items         []BatchWellItemResult `json:"items"`
	Replayed      bool                  `json:"replayed,omitempty"`
}
