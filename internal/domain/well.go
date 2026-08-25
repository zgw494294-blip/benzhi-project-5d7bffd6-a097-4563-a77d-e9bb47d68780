package domain

import (
	"sort"
	"strings"
	"time"
)

type MonitoringWell struct {
	ID                string    `json:"id"`
	CampaignID        string    `json:"campaignId"`
	WellCode          string    `json:"wellCode"`
	LocationLabel     string    `json:"locationLabel"`
	PlannedAnalytes   []string  `json:"plannedAnalytes"`
	ResponsiblePerson string    `json:"responsiblePerson"`
	PlannedSampleAt   time.Time `json:"plannedSampleAt"`
}

type WellPlanDraft struct {
	WellCode          string
	LocationLabel     string
	PlannedAnalytes   []string
	ResponsiblePerson string
	PlannedSampleAt   time.Time
}

func normalizeAnalytes(analytes []string) []string {
	clean := make([]string, 0, len(analytes))
	seen := map[string]bool{}
	for _, a := range analytes {
		a = strings.TrimSpace(a)
		if a != "" && !seen[a] {
			seen[a] = true
			clean = append(clean, a)
		}
	}
	sort.Strings(clean)
	return clean
}

// ValidateWellBatch 聚合单条字段错误与跨条目、既有数据的井号冲突。
func ValidateWellBatch(ids []string, campaignID string, drafts []WellPlanDraft, campaign MonitoringCampaign, existingCodes map[string]bool) ([]MonitoringWell, []ItemFieldError) {
	wells := make([]MonitoringWell, 0, len(drafts))
	issues := []ItemFieldError{}
	seen := map[string]bool{}
	add := func(index int, field, message string) {
		issues = append(issues, ItemFieldError{Index: index, ItemNumber: index + 1, Field: field, Message: message})
	}
	for i, draft := range drafts {
		before := len(issues)
		code := strings.ToUpper(strings.TrimSpace(draft.WellCode))
		location := strings.TrimSpace(draft.LocationLabel)
		person := strings.TrimSpace(draft.ResponsiblePerson)
		analytes := normalizeAnalytes(draft.PlannedAnalytes)
		if i >= len(ids) || ids[i] == "" || campaignID == "" {
			add(i, "id", "井位 ID 和批次 ID 不能为空")
		}
		if code == "" {
			add(i, "wellCode", "监测井编号不能为空")
		} else {
			if seen[code] {
				add(i, "wellCode", "请求内监测井编号重复")
			}
			seen[code] = true
			if existingCodes[code] {
				add(i, "wellCode", "监测井编号已存在")
			}
		}
		if location == "" {
			add(i, "locationLabel", "位置说明不能为空")
		}
		if person == "" {
			add(i, "responsiblePerson", "责任人不能为空")
		}
		if draft.PlannedSampleAt.Before(campaign.SamplingWindowStart) || draft.PlannedSampleAt.After(campaign.SamplingWindowEnd) {
			add(i, "plannedSampleAt", "计划采样时间必须位于批次时间窗内")
		}
		if len(analytes) == 0 {
			add(i, "plannedAnalytes", "至少需要一个计划分析项目")
		}
		if len(issues) == before {
			wells = append(wells, MonitoringWell{ID: ids[i], CampaignID: campaignID, WellCode: code, LocationLabel: location, PlannedAnalytes: analytes, ResponsiblePerson: person, PlannedSampleAt: draft.PlannedSampleAt.UTC()})
		}
	}
	return wells, issues
}

func NewWell(id, campaignID, code, location string, analytes []string, person string, plannedAt time.Time, campaign MonitoringCampaign) (MonitoringWell, error) {
	wells, issues := ValidateWellBatch([]string{id}, campaignID, []WellPlanDraft{{WellCode: code, LocationLabel: location, PlannedAnalytes: analytes, ResponsiblePerson: person, PlannedSampleAt: plannedAt}}, campaign, map[string]bool{})
	if len(issues) > 0 {
		return MonitoringWell{}, FieldError(issues[0].Field, issues[0].Message)
	}
	return wells[0], nil
}
