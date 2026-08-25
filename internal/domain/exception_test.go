package domain

import (
	"testing"
	"time"
)

func TestEvidenceRevisionWithdrawResubmitAndTargetedReview(t *testing.T) {
	now := time.Now().UTC()
	e := QualityException{ID: "e", CampaignID: "c", CheckID: "q", Status: ExceptionOpen, Current: true, EvidenceRevisions: []EvidenceRevision{}}
	revision, err := e.AddEvidence(EvidenceExplanation, "第一版说明", "提交人", 0, now)
	if err != nil || revision != 1 {
		t.Fatalf("提交第一版失败: %v", err)
	}
	if err = e.Withdraw(1, "其他人", now.Add(time.Minute)); err == nil {
		t.Fatal("非提交人撤回应失败")
	}
	if err = e.Withdraw(1, "提交人", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	revision, err = e.AddEvidence(EvidenceExplanation, "第二版说明", "提交人", 1, now.Add(2*time.Minute))
	if err != nil || revision != 2 {
		t.Fatalf("引用撤回版本再提交失败: %v", err)
	}
	if err = e.Review(1, ReviewAccepted, "", "审核员", now.Add(3*time.Minute)); err == nil {
		t.Fatal("审核非最新 revision 应失败")
	}
	if err = e.Review(2, ReviewAccepted, "接受", "审核员", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if e.Status != ExceptionClosed || e.EvidenceRevisions[0].Status != EvidenceWithdrawn || e.EvidenceRevisions[1].Status != EvidenceAccepted {
		t.Fatalf("证据版本状态机不正确: %#v", e)
	}
}
