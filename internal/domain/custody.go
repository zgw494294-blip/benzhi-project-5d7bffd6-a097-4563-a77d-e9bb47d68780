package domain

import (
	"strings"
	"time"
)

type CustodyEvent struct {
	Sequence   int       `json:"sequence"`
	Action     string    `json:"action"`
	FromPerson string    `json:"fromPerson,omitempty"`
	ToPerson   string    `json:"toPerson"`
	OccurredAt time.Time `json:"occurredAt"`
	Condition  string    `json:"condition"`
}

func ValidateCustody(events []CustodyEvent, collectedAt time.Time) error {
	if len(events) == 0 {
		return FieldError("custodyEvents", "至少需要一段交接记录")
	}
	var previous time.Time
	for i, e := range events {
		if e.Sequence != i+1 {
			return FieldError("custodyEvents", "交接序号必须从 1 连续递增")
		}
		if strings.TrimSpace(e.Action) == "" || strings.TrimSpace(e.ToPerson) == "" || strings.TrimSpace(e.Condition) == "" {
			return FieldError("custodyEvents", "交接动作、接收人和状态不能为空")
		}
		if e.OccurredAt.Before(collectedAt) {
			return FieldError("custodyEvents", "交接时间不得早于采集时间")
		}
		if !previous.IsZero() && !e.OccurredAt.After(previous) {
			return FieldError("custodyEvents", "交接时间必须严格递增")
		}
		if i > 0 && strings.TrimSpace(e.FromPerson) != strings.TrimSpace(events[i-1].ToPerson) {
			return FieldError("custodyEvents", "交接链的移交人与前一接收人不连续")
		}
		previous = e.OccurredAt
	}
	return nil
}
