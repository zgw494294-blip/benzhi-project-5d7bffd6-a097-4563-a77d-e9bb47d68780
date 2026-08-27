package checkhistoryresultalias_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

func TestCheckHistoryCacheDoesNotShareNestedResults(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "history-alias.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	app := service.New(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)

	created, err := app.CreateCampaign(ctx, service.CreateCampaignCommand{
		CommandMeta:         service.CommandMeta{IdempotencyKey: "alias-create", Actor: "现场负责人", Role: service.RoleFieldLead},
		CampaignCode:        "ALIAS-20260825",
		SamplingWindowStart: start,
		SamplingWindowEnd:   start.Add(24 * time.Hour),
		RuleSetVersion:      "GW-QC-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	well, err := app.AddWell(ctx, created.CampaignID, service.AddWellCommand{
		CommandMeta:       service.CommandMeta{IdempotencyKey: "alias-well", ExpectedVersion: created.Version, Actor: "现场负责人", Role: service.RoleFieldLead},
		WellCode:          "ALIAS-W01",
		LocationLabel:     "北区",
		PlannedAnalytes:   []string{"pH"},
		ResponsiblePerson: "采样员甲",
		PlannedSampleAt:   start.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.RunQualityCheck(ctx, created.CampaignID, service.RunCheckCommand{CommandMeta: service.CommandMeta{
		IdempotencyKey: "alias-check-1", ExpectedVersion: well.Version, Actor: "质量审核员", Role: service.RoleQualityReviewer,
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.RunQualityCheck(ctx, created.CampaignID, service.RunCheckCommand{CommandMeta: service.CommandMeta{
		IdempotencyKey: "alias-check-2", ExpectedVersion: first.Version, Actor: "质量审核员", Role: service.RoleQualityReviewer,
	}})
	if err != nil {
		t.Fatal(err)
	}

	history, err := app.CheckHistory(ctx, created.CampaignID, 20, 0, first.Check.ID, second.Check.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Items) == 0 || len(history.Items[0].Results) == 0 {
		t.Fatal("测试前置条件缺少质量规则结果")
	}
	original := history.Items[0].Results[0].Message
	history.Items[0].Results[0].Message = "调用方写入的缓存污染标记"

	again, err := app.CheckHistory(ctx, created.CampaignID, 20, 0, first.Check.ID, second.Check.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Items[0].Results[0].Message; got != original {
		t.Fatalf("第二个调用方读到了第一个调用方对嵌套结果的修改: got %q, want %q", got, original)
	}
}
