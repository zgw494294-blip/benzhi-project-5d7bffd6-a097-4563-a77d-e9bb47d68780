package domain

type CampaignStatus string

const (
	StatusDraft     CampaignStatus = "DRAFT"
	StatusRecording CampaignStatus = "RECORDING"
	StatusChecked   CampaignStatus = "CHECKED"
	StatusApproved  CampaignStatus = "APPROVED"
	StatusFrozen    CampaignStatus = "FROZEN"
	StatusReleased  CampaignStatus = "RELEASED"
)

type EvidenceStatus string

const (
	EvidencePending   EvidenceStatus = "PENDING_REVIEW"
	EvidenceWithdrawn EvidenceStatus = "WITHDRAWN"
	EvidenceAccepted  EvidenceStatus = "ACCEPTED"
	EvidenceRejected  EvidenceStatus = "REJECTED"
)

func (s CampaignStatus) CanEditFacts() bool {
	return s == StatusDraft || s == StatusRecording
}

func (s CampaignStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusRecording, StatusChecked, StatusApproved, StatusFrozen, StatusReleased:
		return true
	default:
		return false
	}
}

type ExceptionStatus string

const (
	ExceptionOpen     ExceptionStatus = "OPEN"
	ExceptionPending  ExceptionStatus = "PENDING_REVIEW"
	ExceptionClosed   ExceptionStatus = "CLOSED"
	ExceptionRejected ExceptionStatus = "REJECTED"
)

type ReviewDecision string

const (
	ReviewAccepted ReviewDecision = "ACCEPTED"
	ReviewRejected ReviewDecision = "REJECTED"
)
