package readiness_actor_cache_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/httpapi"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

type readinessEnvelope struct {
	Data service.ApprovalReadiness `json:"data"`
}

func TestApprovalReadinessCacheIsolatesActor(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "readiness.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	app := service.New(repo)
	start := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	created, err := app.CreateCampaign(context.Background(), service.CreateCampaignCommand{
		CommandMeta:         service.CommandMeta{IdempotencyKey: "create", Actor: "现场负责人", Role: service.RoleFieldLead},
		CampaignCode:        "CACHE-ACTOR-1",
		SamplingWindowStart: start,
		SamplingWindowEnd:   start.Add(24 * time.Hour),
		RuleSetVersion:      "GW-QC-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	well, err := app.AddWell(context.Background(), created.CampaignID, service.AddWellCommand{
		CommandMeta:       service.CommandMeta{IdempotencyKey: "well", ExpectedVersion: created.Version, Actor: "现场负责人", Role: service.RoleFieldLead},
		WellCode:          "W-01",
		LocationLabel:     "北区",
		PlannedAnalytes:   []string{"pH"},
		ResponsiblePerson: "采样员",
		PlannedSampleAt:   start.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	version := well.Version
	for i, item := range []struct {
		code string
		kind domain.SampleKind
		well string
	}{
		{code: "S-NORMAL", kind: domain.SampleNormal, well: well.ResourceID},
		{code: "S-DUPLICATE", kind: domain.SampleDuplicate, well: well.ResourceID},
		{code: "S-BLANK", kind: domain.SampleFieldBlank},
	} {
		collected := start.Add(time.Duration(i+2) * time.Hour)
		result, addErr := app.AddSample(context.Background(), created.CampaignID, service.AddSampleCommand{
			CommandMeta:           service.CommandMeta{IdempotencyKey: "sample-" + item.code, ExpectedVersion: version, Actor: "采样员", Role: service.RoleFieldLead},
			WellID:                item.well,
			SampleCode:            item.code,
			SampleKind:            item.kind,
			CollectedAt:           collected,
			FieldMeasurements:     []domain.FieldMeasurement{{Name: "pH", Value: 7, Unit: "pH", MeasuredAt: collected}},
			PreservationMethod:    "4℃冷藏",
			PreservationExpiresAt: start.Add(20 * time.Hour),
			CustodyEvents:         []domain.CustodyEvent{{Sequence: 1, Action: "移交", ToPerson: "接收员", OccurredAt: collected.Add(time.Hour), Condition: "完好"}},
		})
		if addErr != nil {
			t.Fatal(addErr)
		}
		version = result.Version
	}
	checked, err := app.RunQualityCheck(context.Background(), created.CampaignID, service.RunCheckCommand{CommandMeta: service.CommandMeta{
		IdempotencyKey: "check", ExpectedVersion: version, Actor: "质量审核员", Role: service.RoleQualityReviewer,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(checked.Exceptions) != 0 {
		t.Fatalf("测试前置条件错误：质量检查仍有 %d 个异常", len(checked.Exceptions))
	}

	handler := httpapi.New(app).Handler()
	readiness := func(actor string) service.ApprovalReadiness {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+created.CampaignID+"/approval-readiness?actor="+url.QueryEscape(actor), nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("查询就绪状态返回 HTTP %d: %s", response.Code, response.Body.String())
		}
		var envelope readinessEnvelope
		if decodeErr := json.Unmarshal(response.Body.Bytes(), &envelope); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return envelope.Data
	}

	checkerView := readiness("质量审核员")
	if checkerView.Ready {
		t.Fatal("检查执行人应被职责分离规则阻断")
	}
	approverView := readiness("独立技术批准人")
	if !approverView.Ready || len(approverView.Blockers) != 0 {
		t.Fatalf("独立批准人的查询复用了检查执行人的缓存结果: %#v", approverView)
	}
}
