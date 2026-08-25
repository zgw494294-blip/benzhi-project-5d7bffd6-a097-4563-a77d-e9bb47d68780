package service

import (
	"strings"

	"groundwater-release/internal/domain"
)

const (
	RoleFieldLead         = "FIELD_LEAD"
	RoleLabReceiver       = "LAB_RECEIVER"
	RoleQualityReviewer   = "QUALITY_REVIEWER"
	RoleTechnicalApprover = "TECHNICAL_APPROVER"
	RoleReleaseOfficer    = "RELEASE_OFFICER"
)

func authorize(meta CommandMeta, roles ...string) error {
	if strings.TrimSpace(meta.Actor) == "" {
		return domain.FieldError("actor", "操作人不能为空")
	}
	for _, role := range roles {
		if meta.Role == role {
			return nil
		}
	}
	return domain.NewError(domain.ErrForbidden, "当前角色无权执行该操作")
}
