package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/store"
)

func TestCreateCampaignIdempotencyAndVersion(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := New(repo)
	start := time.Now().UTC()
	cmd := CreateCampaignCommand{CommandMeta: CommandMeta{IdempotencyKey: "key-1", Actor: "负责人", Role: RoleFieldLead}, CampaignCode: "TEST-1", SamplingWindowStart: start, SamplingWindowEnd: start.Add(24 * time.Hour), RuleSetVersion: "GW-QC-1"}
	first, err := app.CreateCampaign(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateCampaign(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || first.CampaignID != second.CampaignID {
		t.Fatalf("幂等重放结果不正确: %#v %#v", first, second)
	}
	_, err = app.AddWell(context.Background(), first.CampaignID, AddWellCommand{CommandMeta: CommandMeta{IdempotencyKey: "well-1", ExpectedVersion: 99, Actor: "负责人", Role: RoleFieldLead}, WellCode: "W", LocationLabel: "北区", PlannedAnalytes: []string{"pH"}, ResponsiblePerson: "采样员", PlannedSampleAt: start.Add(time.Hour)})
	if err == nil {
		t.Fatal("过期版本应被拒绝")
	}
}
