package domain

import (
	"strings"
	"time"
)

type MonitoringCampaign struct {
	ID                  string         `json:"id"`
	CampaignCode        string         `json:"campaignCode"`
	SamplingWindowStart time.Time      `json:"samplingWindowStart"`
	SamplingWindowEnd   time.Time      `json:"samplingWindowEnd"`
	RuleSetVersion      string         `json:"ruleSetVersion"`
	Status              CampaignStatus `json:"status"`
	ExpectedVersion     int64          `json:"expectedVersion"`
	FactsRevision       int64          `json:"factsRevision"`
	CreatedAt           time.Time      `json:"createdAt"`
	ApprovedAt          *time.Time     `json:"approvedAt,omitempty"`
	ApprovedCheckID     string         `json:"approvedCheckId,omitempty"`
	ApprovedCheckDigest string         `json:"approvedCheckDigest,omitempty"`
}

func NewCampaign(id, code string, start, end time.Time, ruleVersion string, now time.Time) (MonitoringCampaign, error) {
	code = strings.TrimSpace(code)
	ruleVersion = strings.TrimSpace(ruleVersion)
	if id == "" {
		return MonitoringCampaign{}, FieldError("id", "批次 ID 不能为空")
	}
	if code == "" {
		return MonitoringCampaign{}, FieldError("campaignCode", "批次编号不能为空")
	}
	if !start.Before(end) {
		return MonitoringCampaign{}, FieldError("samplingWindowEnd", "采样结束时间必须晚于开始时间")
	}
	if ruleVersion != "GW-QC-1" {
		return MonitoringCampaign{}, FieldError("ruleSetVersion", "仅支持规则版本 GW-QC-1")
	}
	now = now.UTC()
	return MonitoringCampaign{ID: id, CampaignCode: code, SamplingWindowStart: start.UTC(), SamplingWindowEnd: end.UTC(), RuleSetVersion: ruleVersion, Status: StatusDraft, ExpectedVersion: 1, FactsRevision: 0, CreatedAt: now}, nil
}

func (c *MonitoringCampaign) EnsureVersion(expected int64) error {
	if expected != c.ExpectedVersion {
		return &DomainError{Code: ErrStaleVersion, Message: "expectedVersion 与当前版本不一致", Field: "expectedVersion"}
	}
	return nil
}

func (c *MonitoringCampaign) Touch(facts bool) {
	c.ExpectedVersion++
	if facts {
		c.FactsRevision++
	}
}

func (c *MonitoringCampaign) BeginRecording() error {
	if c.Status == StatusDraft {
		c.Status = StatusRecording
		return nil
	}
	if c.Status == StatusRecording {
		return nil
	}
	return NewError(ErrState, "当前批次状态不允许登记采样事实")
}

func (c *MonitoringCampaign) MarkChecked() error {
	if c.Status != StatusRecording && c.Status != StatusChecked {
		return NewError(ErrState, "仅记录中的批次可以执行质量检查")
	}
	c.Status = StatusChecked
	return nil
}

func (c *MonitoringCampaign) ReopenForRevision() error {
	if c.Status != StatusChecked {
		return NewError(ErrState, "仅已检查且尚未技术批准的批次可以退回修订")
	}
	c.Status = StatusRecording
	return nil
}

func (c *MonitoringCampaign) Approve(at time.Time) error {
	if c.Status != StatusChecked {
		return NewError(ErrState, "仅已检查批次可以技术批准")
	}
	t := at.UTC()
	c.ApprovedAt = &t
	c.Status = StatusApproved
	return nil
}

func (c *MonitoringCampaign) Freeze() error {
	if c.Status != StatusApproved {
		return NewError(ErrState, "仅已技术批准批次可以冻结")
	}
	c.Status = StatusFrozen
	return nil
}

func (c *MonitoringCampaign) Release() error {
	if c.Status != StatusFrozen {
		return NewError(ErrState, "仅冻结批次可以签发凭据")
	}
	c.Status = StatusReleased
	return nil
}
