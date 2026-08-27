package samplevalidationrollback_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"

	_ "modernc.org/sqlite"
)

func TestSampleValidationCacheDoesNotSurviveRollback(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rollback.db")
	repo, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	app := service.New(repo)
	start := time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)
	created, err := app.CreateCampaign(context.Background(), service.CreateCampaignCommand{
		CommandMeta: service.CommandMeta{
			IdempotencyKey: "create-rollback-cache",
			Actor:          "现场负责人",
			Role:           service.RoleFieldLead,
		},
		CampaignCode:        "ROLLBACK-CACHE-1",
		SamplingWindowStart: start,
		SamplingWindowEnd:   start.Add(12 * time.Hour),
		RuleSetVersion:      "GW-QC-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	direct, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer direct.Close()
	_, err = direct.Exec(
		`INSERT INTO audit_events(campaign_id,sequence,event_type,actor,occurred_at,payload,previous_digest,digest) VALUES(?,?,?,?,?,?,?,?)`,
		created.CampaignID, 3, "BLOCK_NEXT_APPEND", "复现器", start.Format(time.RFC3339Nano), []byte(`{}`), "unused", "rollback-cache-blocker",
	)
	if err != nil {
		t.Fatal(err)
	}

	valid := service.AddSampleCommand{
		CommandMeta: service.CommandMeta{
			IdempotencyKey:  "sample-that-rolls-back",
			ExpectedVersion: created.Version,
			Actor:           "现场负责人",
			Role:            service.RoleFieldLead,
		},
		SampleCode:            "ROLLBACK-SAMPLE-1",
		SampleKind:            domain.SampleFieldBlank,
		CollectedAt:           start.Add(time.Hour),
		PreservationMethod:    "冷藏",
		PreservationExpiresAt: start.Add(10 * time.Hour),
		CustodyEvents: []domain.CustodyEvent{{
			Sequence:   1,
			Action:     "移交",
			ToPerson:   "实验室接收员",
			OccurredAt: start.Add(2 * time.Hour),
			Condition:  "完好",
		}},
	}
	if _, err = app.AddSample(context.Background(), created.CampaignID, valid); err == nil {
		t.Fatal("审计序号冲突应使首次样品事务回滚")
	}
	if _, err = direct.Exec(`DELETE FROM audit_events WHERE campaign_id=? AND sequence=3`, created.CampaignID); err != nil {
		t.Fatal(err)
	}

	invalid := valid
	invalid.IdempotencyKey = "invalid-sample-after-rollback"
	invalid.SampleKind = domain.SampleKind("INVALID")
	invalid.PreservationMethod = ""
	invalid.CustodyEvents = nil
	_, err = app.AddSample(context.Background(), created.CampaignID, invalid)
	if err != nil {
		return
	}
	detail, detailErr := app.CampaignDetail(context.Background(), created.CampaignID)
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	if len(detail.Samples) != 1 || detail.Samples[0].SampleKind != domain.SampleKind("INVALID") {
		t.Fatalf("非法样品请求被报告成功，但持久化结果异常: %#v", detail.Samples)
	}
	t.Fatal("事务回滚后复用 sampleCode 跳过领域校验，非法样品已持久化")
}
