package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

type VerificationFailure struct {
	SerialNumber  int64  `json:"serialNumber,omitempty"`
	AuditSequence int64  `json:"auditSequence,omitempty"`
	Code          string `json:"code"`
	Expected      string `json:"expected,omitempty"`
	Actual        string `json:"actual,omitempty"`
	Message       string `json:"message"`
}

type VerificationItems struct {
	SequenceChain     bool `json:"sequenceChain"`
	DatasetDigests    bool `json:"datasetDigests"`
	CredentialDigests bool `json:"credentialDigests"`
	AuditChain        bool `json:"auditChain"`
}

type FullChainVerification struct {
	CampaignID   string               `json:"campaignId"`
	CredentialID string               `json:"credentialId"`
	StartSerial  int64                `json:"startSerial"`
	EndSerial    int64                `json:"endSerial"`
	CheckedCount int                  `json:"checkedCount"`
	Valid        bool                 `json:"valid"`
	Items        VerificationItems    `json:"items"`
	FirstFailure *VerificationFailure `json:"firstFailure,omitempty"`
}

func VerifyCredentialChain(target domain.ReleaseCredential, credentials []domain.ReleaseCredential, datasets map[string]domain.FrozenDataset, events []store.AuditEvent) FullChainVerification {
	r := FullChainVerification{CampaignID: target.CampaignID, CredentialID: target.ID, StartSerial: 1, EndSerial: target.SerialNumber, CheckedCount: len(credentials), Valid: true, Items: VerificationItems{SequenceChain: true, DatasetDigests: true, CredentialDigests: true, AuditChain: true}}
	fail := func(serial int64, code, expected, actual, message string) {
		if r.FirstFailure == nil {
			r.FirstFailure = &VerificationFailure{SerialNumber: serial, Code: code, Expected: expected, Actual: actual, Message: message}
		}
		r.Valid = false
	}
	previous := ""
	for i, c := range credentials {
		expectedSerial := int64(i + 1)
		if c.SerialNumber != expectedSerial {
			r.Items.SequenceChain = false
			fail(c.SerialNumber, "CREDENTIAL_SEQUENCE_GAP", fmt.Sprint(expectedSerial), fmt.Sprint(c.SerialNumber), "凭据序号不连续")
			break
		}
		if c.PreviousDigest != previous {
			r.Items.SequenceChain = false
			fail(c.SerialNumber, "PREVIOUS_DIGEST_MISMATCH", previous, c.PreviousDigest, "凭据前序摘要不匹配")
			break
		}
		d, ok := datasets[c.CampaignID]
		if !ok {
			r.Items.DatasetDigests = false
			fail(c.SerialNumber, "DATASET_MISSING", c.DatasetDigest, "", "凭据对应的冻结数据集缺失")
			break
		}
		sum := sha256.Sum256(d.Content)
		actualDataset := hex.EncodeToString(sum[:])
		if actualDataset != d.Digest {
			r.Items.DatasetDigests = false
			fail(c.SerialNumber, "DATASET_DIGEST_MISMATCH", d.Digest, actualDataset, "冻结数据内容摘要不匹配")
			break
		}
		if c.DatasetDigest != d.Digest || c.DatasetVersion != d.DatasetVersion || c.CampaignID != d.CampaignID {
			r.Items.DatasetDigests = false
			fail(c.SerialNumber, "CREDENTIAL_DATASET_MISMATCH", d.Digest, c.DatasetDigest, "凭据引用的冻结数据集不匹配")
			break
		}
		content := credentialContent{ID: c.ID, CampaignID: c.CampaignID, SerialNumber: c.SerialNumber, DatasetVersion: c.DatasetVersion, DatasetDigest: c.DatasetDigest, PreviousDigest: c.PreviousDigest, IssuedAt: c.IssuedAt.UTC().Format(time.RFC3339Nano), IssuedBy: c.IssuedBy}
		expectedDigest, err := digestJSON(content)
		if err != nil {
			r.Items.CredentialDigests = false
			fail(c.SerialNumber, "CREDENTIAL_DIGEST_ERROR", "", c.CredentialDigest, err.Error())
			break
		}
		if expectedDigest != c.CredentialDigest {
			r.Items.CredentialDigests = false
			fail(c.SerialNumber, "CREDENTIAL_DIGEST_MISMATCH", expectedDigest, c.CredentialDigest, "凭据内容摘要不匹配")
			break
		}
		previous = c.CredentialDigest
	}
	if r.Valid && len(credentials) != int(target.SerialNumber) {
		r.Items.SequenceChain = false
		fail(target.SerialNumber, "CREDENTIAL_SEQUENCE_GAP", fmt.Sprint(target.SerialNumber), fmt.Sprint(len(credentials)), "目标凭据之前的记录缺失")
	}
	if chainFailure, err := VerifyChainDetailed(events); err != nil {
		r.Items.AuditChain = false
		if r.FirstFailure == nil {
			fail(target.SerialNumber, chainFailure.Code, chainFailure.Expected, chainFailure.Actual, chainFailure.Message)
			r.FirstFailure.AuditSequence = chainFailure.Sequence
		} else {
			r.Valid = false
		}
	}
	if r.Valid {
		if chainFailure := verifyCredentialIssuedEvent(events, target); chainFailure != nil {
			r.Items.AuditChain = false
			fail(target.SerialNumber, chainFailure.Code, chainFailure.Expected, chainFailure.Actual, chainFailure.Message)
			r.FirstFailure.AuditSequence = chainFailure.Sequence
		}
	}
	return r
}

type credentialIssuedPayload struct {
	CredentialID     string `json:"credentialId"`
	SerialNumber     int64  `json:"serialNumber"`
	CredentialDigest string `json:"credentialDigest"`
}

func verifyCredentialIssuedEvent(events []store.AuditEvent, target domain.ReleaseCredential) *ChainFailure {
	if len(events) == 0 {
		return nil
	}
	tail := events[len(events)-1]
	if tail.EventType != "CREDENTIAL_ISSUED" {
		return &ChainFailure{Sequence: tail.Sequence, Code: "CREDENTIAL_ISSUED_MISSING", Expected: "CREDENTIAL_ISSUED", Actual: tail.EventType, Message: "审计链尾事件类型不是 CREDENTIAL_ISSUED"}
	}
	var payload credentialIssuedPayload
	if err := json.Unmarshal(tail.Payload, &payload); err != nil {
		return &ChainFailure{Sequence: tail.Sequence, Code: "CREDENTIAL_ISSUED_PAYLOAD_INVALID", Expected: "可解析的 credentialId/serialNumber/credentialDigest", Actual: string(tail.Payload), Message: "凭据签发事件 payload 无法解析"}
	}
	if payload.CredentialID != target.ID {
		return &ChainFailure{Sequence: tail.Sequence, Code: "CREDENTIAL_ISSUED_MISMATCH", Expected: target.ID, Actual: payload.CredentialID, Message: "审计尾事件 credentialId 与目标凭据不一致"}
	}
	if payload.SerialNumber != target.SerialNumber {
		return &ChainFailure{Sequence: tail.Sequence, Code: "CREDENTIAL_ISSUED_MISMATCH", Expected: fmt.Sprint(target.SerialNumber), Actual: fmt.Sprint(payload.SerialNumber), Message: "审计尾事件 serialNumber 与目标凭据不一致"}
	}
	if payload.CredentialDigest != target.CredentialDigest {
		return &ChainFailure{Sequence: tail.Sequence, Code: "CREDENTIAL_ISSUED_MISMATCH", Expected: target.CredentialDigest, Actual: payload.CredentialDigest, Message: "审计尾事件 credentialDigest 与目标凭据不一致"}
	}
	return nil
}

type Verification struct {
	AuditChainValid    bool   `json:"auditChainValid"`
	DatasetDigestValid bool   `json:"datasetDigestValid"`
	CredentialValid    bool   `json:"credentialValid"`
	Message            string `json:"message"`
}

func Verify(snapshot store.CampaignSnapshot, previous *domain.ReleaseCredential) Verification {
	v := Verification{AuditChainValid: true, DatasetDigestValid: true, CredentialValid: true, Message: "完整性校验通过"}
	if err := VerifyChain(snapshot.Timeline); err != nil {
		v.AuditChainValid = false
		v.Message = err.Error()
	}
	if snapshot.Dataset != nil {
		sum := sha256.Sum256(snapshot.Dataset.Content)
		if hex.EncodeToString(sum[:]) != snapshot.Dataset.Digest {
			v.DatasetDigestValid = false
			v.Message = "冻结数据集摘要不匹配"
		}
	}
	if snapshot.Credential != nil {
		if snapshot.Dataset == nil {
			v.CredentialValid = false
			v.Message = "凭据缺少冻结数据集"
		} else if err := VerifyCredential(*snapshot.Credential, *snapshot.Dataset, previous); err != nil {
			v.CredentialValid = false
			v.Message = err.Error()
		}
	}
	return v
}
