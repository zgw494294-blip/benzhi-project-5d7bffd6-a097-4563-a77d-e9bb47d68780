package detail_cache_stale_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

func TestCampaignDetailRefreshesAfterMutation(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "detail-cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	app := service.New(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC)
	created, err := app.CreateCampaign(ctx, service.CreateCampaignCommand{
		CommandMeta:         service.CommandMeta{IdempotencyKey: "create-cache-case", Actor: "现场负责人", Role: service.RoleFieldLead},
		CampaignCode:        "CACHE-EDGE-1",
		SamplingWindowStart: start,
		SamplingWindowEnd:   start.Add(24 * time.Hour),
		RuleSetVersion:      "GW-QC-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	before, err := app.CampaignDetail(ctx, created.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Wells) != 0 {
		t.Fatalf("初始详情不应包含监测井: %#v", before.Wells)
	}

	registered, err := app.AddWell(ctx, created.CampaignID, service.AddWellCommand{
		CommandMeta:       service.CommandMeta{IdempotencyKey: "add-well-after-detail", ExpectedVersion: created.Version, Actor: "现场负责人", Role: service.RoleFieldLead},
		WellCode:          "W-CACHE-1",
		LocationLabel:     "北区",
		PlannedAnalytes:   []string{"pH"},
		ResponsiblePerson: "采样员甲",
		PlannedSampleAt:   start.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	after, err := app.CampaignDetail(ctx, created.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Campaign.ExpectedVersion != registered.Version || len(after.Wells) != 1 || after.Wells[0].ID != registered.ResourceID {
		t.Fatalf("写事务完成后详情仍复用了旧缓存: version=%d wells=%d", after.Campaign.ExpectedVersion, len(after.Wells))
	}
}
