package domain

import "time"

type RuleSeverity string

const (
	SeverityError   RuleSeverity = "ERROR"
	SeverityWarning RuleSeverity = "WARNING"
)

type QualityResult struct {
	RuleCode  string       `json:"ruleCode"`
	Passed    bool         `json:"passed"`
	Severity  RuleSeverity `json:"severity"`
	SubjectID string       `json:"subjectId,omitempty"`
	Message   string       `json:"message"`
}

type QualityCheck struct {
	ID             string          `json:"id"`
	CampaignID     string          `json:"campaignId"`
	RuleSetVersion string          `json:"ruleSetVersion"`
	FactsRevision  int64           `json:"factsRevision"`
	Results        []QualityResult `json:"results"`
	ResultDigest   string          `json:"resultDigest"`
	CheckedAt      time.Time       `json:"checkedAt"`
	CheckedBy      string          `json:"checkedBy"`
	Current        bool            `json:"current"`
	InvalidatedAt  *time.Time      `json:"invalidatedAt,omitempty"`
	InvalidReason  string          `json:"invalidReason,omitempty"`
}

func (q QualityCheck) Passed() bool {
	for _, r := range q.Results {
		if !r.Passed && r.Severity == SeverityError {
			return false
		}
	}
	return true
}

func (q QualityCheck) Fresh(c MonitoringCampaign) bool {
	return q.CampaignID == c.ID && q.FactsRevision == c.FactsRevision && q.RuleSetVersion == c.RuleSetVersion
}
