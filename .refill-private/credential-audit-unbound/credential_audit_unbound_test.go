package credentialauditunbound

import (
	"testing"
	"time"

	"groundwater-release/internal/audit"
	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

func TestCredentialVerificationBindsAuditTailToCredential(t *testing.T) {
	now := time.Date(2026, 3, 4, 8, 0, 0, 0, time.UTC)
	campaign := domain.MonitoringCampaign{
		ID: "campaign-1", CampaignCode: "AUDIT-TAIL", RuleSetVersion: "GW-QC-1",
		Status: domain.StatusFrozen, ExpectedVersion: 5, FactsRevision: 1, CreatedAt: now,
		SamplingWindowStart: now, SamplingWindowEnd: now.Add(24 * time.Hour),
	}
	dataset, err := domain.BuildFrozenDataset(campaign, nil, nil, domain.QualityCheck{
		ID: "check-1", CampaignID: campaign.ID, RuleSetVersion: campaign.RuleSetVersion,
		FactsRevision: campaign.FactsRevision, ResultDigest: "check-digest", CheckedAt: now, CheckedBy: "质量审核员",
	}, nil, 1, "技术批准员", now)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := audit.IssueCredential("credential-1", dataset, 1, "", "放行员", now)
	if err != nil {
		t.Fatal(err)
	}
	wrongTail, err := audit.AppendEvent(nil, campaign.ID, "CREDENTIAL_ISSUED", "放行员", map[string]any{
		"credentialId": "different-credential", "serialNumber": credential.SerialNumber,
		"credentialDigest": credential.CredentialDigest,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	report := audit.VerifyCredentialChain(
		credential,
		[]domain.ReleaseCredential{credential},
		map[string]domain.FrozenDataset{campaign.ID: dataset},
		[]store.AuditEvent{wrongTail},
	)
	if report.Valid || report.Items.AuditChain || report.FirstFailure == nil {
		t.Fatalf("TestCredentialVerificationBindsAuditTailToCredential: unrelated credential audit tail accepted: %#v", report)
	}
}
