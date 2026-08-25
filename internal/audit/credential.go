package audit

import (
	"fmt"
	"time"

	"groundwater-release/internal/domain"
)

type credentialContent struct {
	ID             string `json:"id"`
	CampaignID     string `json:"campaignId"`
	SerialNumber   int64  `json:"serialNumber"`
	DatasetVersion int64  `json:"datasetVersion"`
	DatasetDigest  string `json:"datasetDigest"`
	PreviousDigest string `json:"previousDigest"`
	IssuedAt       string `json:"issuedAt"`
	IssuedBy       string `json:"issuedBy"`
}

func IssueCredential(id string, dataset domain.FrozenDataset, serial int64, previousDigest, actor string, now time.Time) (domain.ReleaseCredential, error) {
	if id == "" || actor == "" {
		return domain.ReleaseCredential{}, domain.FieldError("issuedBy", "凭据 ID 和签发人不能为空")
	}
	if serial < 1 {
		return domain.ReleaseCredential{}, domain.NewError(domain.ErrIntegrity, "凭据序号必须为正数")
	}
	if !validHexDigest(dataset.Digest) {
		return domain.ReleaseCredential{}, domain.NewError(domain.ErrIntegrity, "冻结数据集摘要格式无效")
	}
	now = now.UTC()
	content := credentialContent{ID: id, CampaignID: dataset.CampaignID, SerialNumber: serial, DatasetVersion: dataset.DatasetVersion, DatasetDigest: dataset.Digest, PreviousDigest: previousDigest, IssuedAt: now.Format(time.RFC3339Nano), IssuedBy: actor}
	digest, err := digestJSON(content)
	if err != nil {
		return domain.ReleaseCredential{}, err
	}
	return domain.ReleaseCredential{ID: id, CampaignID: dataset.CampaignID, SerialNumber: serial, DatasetVersion: dataset.DatasetVersion, DatasetDigest: dataset.Digest, PreviousDigest: previousDigest, CredentialDigest: digest, IssuedAt: now, IssuedBy: actor}, nil
}

func VerifyCredential(c domain.ReleaseCredential, dataset domain.FrozenDataset, previous *domain.ReleaseCredential) error {
	if c.CampaignID != dataset.CampaignID || c.DatasetVersion != dataset.DatasetVersion || c.DatasetDigest != dataset.Digest {
		return fmt.Errorf("凭据引用的冻结数据集不匹配")
	}
	expectedPrevious := ""
	if c.SerialNumber > 1 {
		if previous == nil || previous.SerialNumber != c.SerialNumber-1 {
			return fmt.Errorf("凭据前序记录缺失")
		}
		expectedPrevious = previous.CredentialDigest
	}
	if c.PreviousDigest != expectedPrevious {
		return fmt.Errorf("凭据前序摘要不匹配")
	}
	content := credentialContent{ID: c.ID, CampaignID: c.CampaignID, SerialNumber: c.SerialNumber, DatasetVersion: c.DatasetVersion, DatasetDigest: c.DatasetDigest, PreviousDigest: c.PreviousDigest, IssuedAt: c.IssuedAt.UTC().Format(time.RFC3339Nano), IssuedBy: c.IssuedBy}
	digest, err := digestJSON(content)
	if err != nil {
		return err
	}
	if digest != c.CredentialDigest {
		return fmt.Errorf("凭据内容摘要不匹配")
	}
	return nil
}
