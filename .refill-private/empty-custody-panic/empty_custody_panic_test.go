package emptycustodypanic

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

func TestQualityCheckHandlesEmptyCustodyWithoutPanic(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "quality.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	app := service.New(repo)
	start := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	created, err := app.CreateCampaign(context.Background(), service.CreateCampaignCommand{
		CommandMeta:         service.CommandMeta{IdempotencyKey: "create", Actor: "现场负责人", Role: service.RoleFieldLead},
		CampaignCode:        "EMPTY-CUSTODY",
		SamplingWindowStart: start,
		SamplingWindowEnd:   start.Add(24 * time.Hour),
		RuleSetVersion:      "GW-QC-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	well, err := app.AddWell(context.Background(), created.CampaignID, service.AddWellCommand{
		CommandMeta:       service.CommandMeta{IdempotencyKey: "well", ExpectedVersion: created.Version, Actor: "现场负责人", Role: service.RoleFieldLead},
		WellCode:          "W-1",
		LocationLabel:     "北区",
		PlannedAnalytes:   []string{"pH"},
		ResponsiblePerson: "采样员",
		PlannedSampleAt:   start.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Execute(context.Background(), "inject", "inject-empty-custody", func(tx *store.TxStore) (json.RawMessage, error) {
		err := tx.InsertSample(domain.SampleRecord{
			ID:                    "sample-empty-custody",
			CampaignID:            created.CampaignID,
			WellID:                well.ResourceID,
			SampleCode:            "S-EMPTY",
			SampleKind:            domain.SampleNormal,
			CollectedAt:           start.Add(2 * time.Hour),
			FieldMeasurements:     []domain.FieldMeasurement{},
			PreservationMethod:    "冷藏",
			PreservationExpiresAt: start.Add(20 * time.Hour),
			CustodyEvents:         []domain.CustodyEvent{},
			Revision:              1,
		})
		return json.RawMessage(`{}`), err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("TestQualityCheckHandlesEmptyCustodyWithoutPanic: panic: %v", recovered)
		}
	}()
	_, _ = app.RunQualityCheck(context.Background(), created.CampaignID, service.RunCheckCommand{CommandMeta: service.CommandMeta{
		IdempotencyKey: "check", ExpectedVersion: well.Version, Actor: "质量审核员", Role: service.RoleQualityReviewer,
	}})
}
