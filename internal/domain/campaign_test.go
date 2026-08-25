package domain

import (
	"testing"
	"time"
)

func TestCampaignStateMachine(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, err := NewCampaign("c1", "C-1", start, start.Add(24*time.Hour), "GW-QC-1", start)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.BeginRecording(); err != nil {
		t.Fatal(err)
	}
	if err = c.MarkChecked(); err != nil {
		t.Fatal(err)
	}
	if err = c.Approve(start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = c.Freeze(); err != nil {
		t.Fatal(err)
	}
	if err = c.Release(); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusReleased {
		t.Fatalf("最终状态为 %s", c.Status)
	}
}

func TestCustodyRejectsDiscontinuity(t *testing.T) {
	now := time.Now().UTC()
	events := []CustodyEvent{{Sequence: 1, Action: "移交", ToPerson: "甲", OccurredAt: now.Add(time.Hour), Condition: "完好"}, {Sequence: 2, Action: "移交", FromPerson: "乙", ToPerson: "丙", OccurredAt: now.Add(2 * time.Hour), Condition: "完好"}}
	if err := ValidateCustody(events, now); err == nil {
		t.Fatal("应拒绝不连续交接链")
	}
}
