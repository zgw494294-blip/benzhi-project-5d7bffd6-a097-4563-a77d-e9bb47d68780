package domain

import (
	"testing"
	"time"
)

func TestEvaluateQualityProducesStableBlankFailure(t *testing.T) {
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	campaign, _ := NewCampaign("c", "C", start, start.Add(24*time.Hour), "GW-QC-1", start)
	campaign.FactsRevision = 3
	well := MonitoringWell{ID: "w", CampaignID: "c"}
	custody := []CustodyEvent{{Sequence: 1, Action: "移交", ToPerson: "实验室", OccurredAt: start.Add(3 * time.Hour), Condition: "完好"}}
	samples := []SampleRecord{{ID: "s1", CampaignID: "c", WellID: "w", SampleCode: "S1", SampleKind: SampleNormal, CollectedAt: start.Add(time.Hour), PreservationExpiresAt: start.Add(10 * time.Hour), CustodyEvents: custody}, {ID: "s2", CampaignID: "c", WellID: "w", SampleCode: "S2", SampleKind: SampleDuplicate, CollectedAt: start.Add(time.Hour), PreservationExpiresAt: start.Add(10 * time.Hour), CustodyEvents: custody}}
	first, err := EvaluateQuality(campaign, []MonitoringWell{well}, samples, "q", "审核员", start.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, _ := EvaluateQuality(campaign, []MonitoringWell{well}, samples, "q", "审核员", start.Add(4*time.Hour))
	if first.ResultDigest != second.ResultDigest {
		t.Fatal("相同事实的检查摘要必须稳定")
	}
	failed := 0
	for _, result := range first.Results {
		if !result.Passed {
			failed++
			if result.RuleCode != RuleFieldBlank {
				t.Fatalf("意外失败规则 %s", result.RuleCode)
			}
		}
	}
	if failed != 1 {
		t.Fatalf("失败规则数=%d", failed)
	}
}
