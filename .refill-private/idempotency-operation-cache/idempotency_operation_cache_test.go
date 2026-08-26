package idempotency_operation_cache_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

func TestIdempotencyCacheRejectsCrossOperationReuse(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "reproduction.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	app := service.New(repo)
	start := time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC)
	created, err := app.CreateCampaign(context.Background(), service.CreateCampaignCommand{
		CommandMeta:         service.CommandMeta{IdempotencyKey: "shared-key", Actor: "现场负责人", Role: service.RoleFieldLead},
		CampaignCode:        "CACHE-OP-1",
		SamplingWindowStart: start,
		SamplingWindowEnd:   start.Add(24 * time.Hour),
		RuleSetVersion:      "GW-QC-1",
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	_, err = app.AddWell(context.Background(), created.CampaignID, service.AddWellCommand{
		CommandMeta:       service.CommandMeta{IdempotencyKey: "shared-key", ExpectedVersion: created.Version, Actor: "现场负责人", Role: service.RoleFieldLead},
		WellCode:          "W-CACHE",
		LocationLabel:     "北区",
		PlannedAnalytes:   []string{"pH"},
		ResponsiblePerson: "采样员",
		PlannedSampleAt:   start.Add(time.Hour),
	})
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrConflict {
		t.Fatalf("cross-operation idempotency key reuse must return conflict, got %v", err)
	}

	snapshot, err := repo.Snapshot(context.Background(), created.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Wells) != 0 {
		t.Fatalf("conflicting replay persisted %d wells", len(snapshot.Wells))
	}
}
