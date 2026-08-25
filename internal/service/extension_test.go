package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.Repository) {
	t.Helper()
	repo, err := store.Open(filepath.Join(t.TempDir(), "extension.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return New(repo), repo
}

func createTestCampaign(t *testing.T, app *Service, start time.Time) MutationResult {
	t.Helper()
	result, err := app.CreateCampaign(context.Background(), CreateCampaignCommand{CommandMeta: CommandMeta{IdempotencyKey: "create", Actor: "现场负责人", Role: RoleFieldLead}, CampaignCode: "EXT-1", SamplingWindowStart: start, SamplingWindowEnd: start.Add(24 * time.Hour), RuleSetVersion: "GW-QC-1"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBatchWellsAtomicValidationAndSingleVersionAdvance(t *testing.T) {
	app, repo := newTestService(t)
	start := time.Now().UTC().Truncate(time.Second)
	created := createTestCampaign(t, app, start)
	existing, err := app.AddWell(context.Background(), created.CampaignID, AddWellCommand{CommandMeta: CommandMeta{IdempotencyKey: "existing", ExpectedVersion: created.Version, Actor: "现场负责人", Role: RoleFieldLead}, WellCode: " w-01 ", LocationLabel: "北区", PlannedAnalytes: []string{" pH "}, ResponsiblePerson: "采样员", PlannedSampleAt: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.AddWellsBatch(context.Background(), created.CampaignID, AddWellsBatchCommand{CommandMeta: CommandMeta{IdempotencyKey: "bad-batch", ExpectedVersion: existing.Version, Actor: "现场负责人", Role: RoleFieldLead}, Items: []WellPlan{{WellCode: "W-01", LocationLabel: "重复", PlannedAnalytes: []string{"pH"}, ResponsiblePerson: "甲", PlannedSampleAt: start.Add(2 * time.Hour)}, {WellCode: "W-02", LocationLabel: "越界", PlannedAnalytes: []string{"硝酸盐"}, ResponsiblePerson: "乙", PlannedSampleAt: start.Add(25 * time.Hour)}}})
	var validation *domain.BatchValidationError
	if !errors.As(err, &validation) || len(validation.Items) != 2 {
		t.Fatalf("应同时返回两个逐项错误: %#v", err)
	}
	snapshot, err := repo.Snapshot(context.Background(), created.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Campaign.ExpectedVersion != existing.Version || len(snapshot.Wells) != 1 {
		t.Fatalf("失败批次发生了写入: %#v", snapshot.Campaign)
	}
	good, err := app.AddWellsBatch(context.Background(), created.CampaignID, AddWellsBatchCommand{CommandMeta: CommandMeta{IdempotencyKey: "good-batch", ExpectedVersion: existing.Version, Actor: "现场负责人", Role: RoleFieldLead}, Items: []WellPlan{{WellCode: " w-02 ", LocationLabel: "东区", PlannedAnalytes: []string{"硝酸盐"}, ResponsiblePerson: "乙", PlannedSampleAt: start.Add(2 * time.Hour)}, {WellCode: "W-03", LocationLabel: "南区", PlannedAnalytes: []string{"pH"}, ResponsiblePerson: "丙", PlannedSampleAt: start.Add(3 * time.Hour)}, {WellCode: "W-04", LocationLabel: "西区", PlannedAnalytes: []string{"氨氮"}, ResponsiblePerson: "丁", PlannedSampleAt: start.Add(4 * time.Hour)}}})
	if err != nil {
		t.Fatal(err)
	}
	if good.Version != existing.Version+1 || len(good.Items) != 3 || good.Items[0].Index != 0 {
		t.Fatalf("批量登记结果不符合顺序或单次版本推进要求: %#v", good)
	}
}

func TestCheckHistoryReopenAndSampleRevision(t *testing.T) {
	app, repo := newTestService(t)
	start := time.Now().UTC().Truncate(time.Second)
	created := createTestCampaign(t, app, start)
	well, err := app.AddWell(context.Background(), created.CampaignID, AddWellCommand{CommandMeta: CommandMeta{IdempotencyKey: "well", ExpectedVersion: created.Version, Actor: "现场负责人", Role: RoleFieldLead}, WellCode: "W-1", LocationLabel: "北区", PlannedAnalytes: []string{"pH"}, ResponsiblePerson: "甲", PlannedSampleAt: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	collected := start.Add(2 * time.Hour)
	sample, err := app.AddSample(context.Background(), created.CampaignID, AddSampleCommand{CommandMeta: CommandMeta{IdempotencyKey: "sample", ExpectedVersion: well.Version, Actor: "现场负责人", Role: RoleFieldLead}, WellID: well.ResourceID, SampleCode: "S-1", SampleKind: domain.SampleNormal, CollectedAt: collected, FieldMeasurements: []domain.FieldMeasurement{{Name: "pH", Value: 7, Unit: "pH", MeasuredAt: collected}}, PreservationMethod: "冷藏", PreservationExpiresAt: start.Add(20 * time.Hour), CustodyEvents: []domain.CustodyEvent{{Sequence: 1, Action: "移交", ToPerson: "接收员", OccurredAt: start.Add(3 * time.Hour), Condition: "完好"}}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.RunQualityCheck(context.Background(), created.CampaignID, RunCheckCommand{CommandMeta: CommandMeta{IdempotencyKey: "check-1", ExpectedVersion: sample.Version, Actor: "审核员", Role: RoleQualityReviewer}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.RunQualityCheck(context.Background(), created.CampaignID, RunCheckCommand{CommandMeta: CommandMeta{IdempotencyKey: "check-2", ExpectedVersion: first.Version, Actor: "审核员", Role: RoleQualityReviewer}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Check.ID == second.Check.ID || first.Check.ResultDigest != second.Check.ResultDigest {
		t.Fatal("相同事实重检应保留不同 checkID 和相同摘要")
	}
	readiness, err := app.ApprovalReadiness(context.Background(), created.CampaignID, "审核员")
	if err != nil {
		t.Fatal(err)
	}
	foundSeparation := false
	for _, blocker := range readiness.Blockers {
		if blocker.Code == "CHECKER_APPROVER_CONFLICT" {
			foundSeparation = true
		}
	}
	if readiness.Ready || !foundSeparation {
		t.Fatalf("检查执行人的就绪评估应被职责分离阻断: %#v", readiness)
	}
	_, err = app.Approve(context.Background(), created.CampaignID, ApproveCommand{CommandMeta: CommandMeta{IdempotencyKey: "stale-approve", ExpectedVersion: second.Version, Actor: "技术批准人", Role: RoleTechnicalApprover}, CheckDigest: first.Check.ResultDigest + "-old"})
	if err == nil {
		t.Fatal("旧 checkDigest 应被拒绝")
	}
	history, err := app.CheckHistory(context.Background(), created.CampaignID, 20, 0, first.Check.ID, second.Check.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 2 || history.Difference == nil || len(history.Difference.NewFailures) != 0 || len(history.Difference.ResolvedFailures) != 0 {
		t.Fatalf("检查历史或差异不正确: %#v", history)
	}
	reopened, err := app.ReopenCheck(context.Background(), created.CampaignID, ReopenCheckCommand{CommandMeta: CommandMeta{IdempotencyKey: "reopen", ExpectedVersion: second.Version, Actor: "审核员", Role: RoleQualityReviewer}, Reason: "保存方式录入有误"})
	if err != nil {
		t.Fatal(err)
	}
	method := "4℃冷藏"
	revised, err := app.ReviseSample(context.Background(), created.CampaignID, sample.ResourceID, ReviseSampleCommand{CommandMeta: CommandMeta{IdempotencyKey: "revise", ExpectedVersion: reopened.Version, Actor: "接收员", Role: RoleLabReceiver}, Revision: 1, PreservationMethod: &method, AppendCustodyEvents: []domain.CustodyEvent{{Sequence: 2, Action: "入库", FromPerson: "接收员", ToPerson: "保管员", OccurredAt: start.Add(4 * time.Hour), Condition: "完好"}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.Snapshot(context.Background(), created.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if revised.Version != reopened.Version+1 || snapshot.Samples[0].Revision != 2 || len(snapshot.SampleRevisionHistory[sample.ResourceID]) != 1 || snapshot.QualityCheck != nil {
		t.Fatalf("样品修订历史或检查失效状态不正确: %#v", snapshot)
	}
}
