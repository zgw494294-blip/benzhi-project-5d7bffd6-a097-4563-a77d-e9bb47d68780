package previouscredentialstmtlifetime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

func TestRepeatedCredentialDetailKeepsStatementValid(t *testing.T) {
	repo, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	defer repo.Close()

	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	first := releasedCampaign("campaign-1", "GW-2026-001", now)
	second := releasedCampaign("campaign-2", "GW-2026-002", now.Add(time.Hour))
	_, err = repo.Execute(context.Background(), "seed-two-credentials", "test_setup", func(tx *store.TxStore) (json.RawMessage, error) {
		for _, campaign := range []domain.MonitoringCampaign{first, second} {
			if err := tx.InsertCampaign(campaign); err != nil {
				return nil, err
			}
			dataset := domain.FrozenDataset{
				CampaignID: campaign.ID, DatasetVersion: 1, Content: json.RawMessage(`{"campaignId":"` + campaign.ID + `"}`),
				Digest: "dataset-" + campaign.ID, FrozenAt: now, FrozenBy: "freezer",
			}
			if err := tx.InsertDataset(dataset); err != nil {
				return nil, err
			}
		}
		if err := tx.InsertCredential(domain.ReleaseCredential{
			ID: "credential-1", CampaignID: first.ID, SerialNumber: 1, DatasetVersion: 1,
			DatasetDigest: "dataset-" + first.ID, CredentialDigest: "credential-digest-1", IssuedAt: now, IssuedBy: "issuer",
		}); err != nil {
			return nil, err
		}
		if err := tx.InsertCredential(domain.ReleaseCredential{
			ID: "credential-2", CampaignID: second.ID, SerialNumber: 2, DatasetVersion: 1,
			DatasetDigest: "dataset-" + second.ID, PreviousDigest: "credential-digest-1", CredentialDigest: "credential-digest-2", IssuedAt: now.Add(time.Hour), IssuedBy: "issuer",
		}); err != nil {
			return nil, err
		}
		return json.RawMessage(`{}`), nil
	})
	if err != nil {
		t.Fatalf("准备双凭据状态: %v", err)
	}

	svc := service.New(repo)
	if _, err = svc.CampaignDetail(context.Background(), second.ID); err != nil {
		t.Fatalf("首次查询详情: %v", err)
	}
	_, err = svc.CampaignDetail(context.Background(), second.ID)
	if err != nil {
		if strings.Contains(err.Error(), "statement is closed") {
			t.Fatalf("重复查询复用了已结束事务的 prepared statement: %v", err)
		}
		t.Fatalf("重复查询详情: %v", err)
	}
}

func releasedCampaign(id, code string, now time.Time) domain.MonitoringCampaign {
	return domain.MonitoringCampaign{
		ID: id, CampaignCode: code, SamplingWindowStart: now, SamplingWindowEnd: now.Add(2 * time.Hour),
		RuleSetVersion: "GW-QC-1", Status: domain.StatusReleased, ExpectedVersion: 6, FactsRevision: 2, CreatedAt: now,
	}
}
