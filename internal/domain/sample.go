package domain

import (
	"sort"
	"strings"
	"time"
)

type SampleKind string

const (
	SampleNormal     SampleKind = "NORMAL"
	SampleFieldBlank SampleKind = "FIELD_BLANK"
	SampleDuplicate  SampleKind = "DUPLICATE"
)

type FieldMeasurement struct {
	Name       string    `json:"name"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	MeasuredAt time.Time `json:"measuredAt"`
}

type SampleRecord struct {
	ID                    string             `json:"id"`
	CampaignID            string             `json:"campaignId"`
	WellID                string             `json:"wellId,omitempty"`
	SampleCode            string             `json:"sampleCode"`
	SampleKind            SampleKind         `json:"sampleKind"`
	CollectedAt           time.Time          `json:"collectedAt"`
	FieldMeasurements     []FieldMeasurement `json:"fieldMeasurements"`
	PreservationMethod    string             `json:"preservationMethod"`
	PreservationExpiresAt time.Time          `json:"preservationExpiresAt"`
	CustodyEvents         []CustodyEvent     `json:"custodyEvents"`
	Revision              int64              `json:"revision"`
}

type SampleRevisionSnapshot struct {
	Sample    SampleRecord `json:"sample"`
	RevisedAt time.Time    `json:"revisedAt"`
	RevisedBy string       `json:"revisedBy"`
}

// Revise 只允许修改可纠正的事实；样品身份及既有交接段保持不变。
func (s SampleRecord) Revise(measurements *[]FieldMeasurement, preservationMethod *string, preservationExpiresAt *time.Time, appended []CustodyEvent, expectedRevision int64, campaign MonitoringCampaign, wellExists bool) (SampleRecord, error) {
	if expectedRevision != s.Revision {
		return SampleRecord{}, &DomainError{Code: ErrStaleVersion, Field: "revision", Message: "样品 revision 与当前版本不一致"}
	}
	if measurements != nil {
		s.FieldMeasurements = append([]FieldMeasurement(nil), (*measurements)...)
	}
	if preservationMethod != nil {
		s.PreservationMethod = *preservationMethod
	}
	if preservationExpiresAt != nil {
		s.PreservationExpiresAt = preservationExpiresAt.UTC()
	}
	if len(appended) > 0 {
		s.CustodyEvents = append(append([]CustodyEvent(nil), s.CustodyEvents...), appended...)
	}
	if err := s.Validate(campaign, wellExists); err != nil {
		return SampleRecord{}, err
	}
	s.Revision++
	return s, nil
}

func (s SampleRecord) Validate(c MonitoringCampaign, wellExists bool) error {
	if s.ID == "" || s.CampaignID == "" {
		return FieldError("id", "样品 ID 和批次 ID 不能为空")
	}
	s.SampleCode = strings.TrimSpace(s.SampleCode)
	if s.SampleCode == "" {
		return FieldError("sampleCode", "样品编号不能为空")
	}
	switch s.SampleKind {
	case SampleNormal, SampleFieldBlank, SampleDuplicate:
	default:
		return FieldError("sampleKind", "样品类型无效")
	}
	if s.SampleKind != SampleFieldBlank && (s.WellID == "" || !wellExists) {
		return FieldError("wellId", "常规样和现场平行样必须关联有效监测井")
	}
	if s.CollectedAt.Before(c.SamplingWindowStart) || s.CollectedAt.After(c.SamplingWindowEnd) {
		return FieldError("collectedAt", "采集时间必须位于批次时间窗内")
	}
	if strings.TrimSpace(s.PreservationMethod) == "" {
		return FieldError("preservationMethod", "保存方式不能为空")
	}
	if !s.PreservationExpiresAt.After(s.CollectedAt) {
		return FieldError("preservationExpiresAt", "保存期限必须晚于采集时间")
	}
	for i := range s.FieldMeasurements {
		m := &s.FieldMeasurements[i]
		m.Name = strings.TrimSpace(m.Name)
		m.Unit = strings.TrimSpace(m.Unit)
		if m.Name == "" || m.Unit == "" {
			return FieldError("fieldMeasurements", "现场测量名称和单位不能为空")
		}
		if m.MeasuredAt.Before(s.CollectedAt.Add(-30*time.Minute)) || m.MeasuredAt.After(s.CollectedAt.Add(2*time.Hour)) {
			return FieldError("fieldMeasurements", "现场测量时间与采样时间不合理")
		}
		m.MeasuredAt = m.MeasuredAt.UTC()
	}
	if err := ValidateCustody(s.CustodyEvents, s.CollectedAt); err != nil {
		return err
	}
	sort.Slice(s.FieldMeasurements, func(i, j int) bool { return s.FieldMeasurements[i].Name < s.FieldMeasurements[j].Name })
	return nil
}
