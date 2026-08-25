package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const (
	RuleCompleteness = "GW-COMPLETE-001"
	RuleFieldBlank   = "GW-BLANK-001"
	RuleDuplicate    = "GW-DUPLICATE-001"
	RulePreservation = "GW-PRESERVE-001"
	RuleCustody      = "GW-CUSTODY-001"
)

func EvaluateQuality(c MonitoringCampaign, wells []MonitoringWell, samples []SampleRecord, checkID, actor string, now time.Time) (QualityCheck, error) {
	if c.RuleSetVersion != "GW-QC-1" {
		return QualityCheck{}, NewError(ErrValidation, "不支持的质量规则版本")
	}
	results := make([]QualityResult, 0)
	byWell := map[string][]SampleRecord{}
	blanks := 0
	for _, s := range samples {
		byWell[s.WellID] = append(byWell[s.WellID], s)
		if s.SampleKind == SampleFieldBlank {
			blanks++
		}
	}
	for _, w := range wells {
		normal, duplicate := 0, 0
		for _, s := range byWell[w.ID] {
			if s.SampleKind == SampleNormal {
				normal++
			}
			if s.SampleKind == SampleDuplicate {
				duplicate++
			}
		}
		results = append(results, QualityResult{RuleCode: RuleCompleteness, Passed: normal > 0, Severity: SeverityError, SubjectID: w.ID, Message: choose(normal > 0, "监测井已登记常规样", "监测井缺少常规样")})
		results = append(results, QualityResult{RuleCode: RuleDuplicate, Passed: duplicate > 0, Severity: SeverityError, SubjectID: w.ID, Message: choose(duplicate > 0, "监测井已登记现场平行样", "监测井缺少现场平行样")})
	}
	results = append(results, QualityResult{RuleCode: RuleFieldBlank, Passed: blanks > 0, Severity: SeverityError, Message: choose(blanks > 0, "批次包含现场空白样", "批次缺少现场空白样")})
	for _, s := range samples {
		last := s.CustodyEvents[len(s.CustodyEvents)-1].OccurredAt
		preserved := !last.After(s.PreservationExpiresAt)
		results = append(results, QualityResult{RuleCode: RulePreservation, Passed: preserved, Severity: SeverityError, SubjectID: s.ID, Message: choose(preserved, "交接在保存期限内完成", "交接完成时间超过保存期限")})
		continuous := ValidateCustody(s.CustodyEvents, s.CollectedAt) == nil
		results = append(results, QualityResult{RuleCode: RuleCustody, Passed: continuous, Severity: SeverityError, SubjectID: s.ID, Message: choose(continuous, "交接链连续", "交接链不连续")})
	}
	if len(wells) == 0 {
		results = append(results, QualityResult{RuleCode: RuleCompleteness, Passed: false, Severity: SeverityError, Message: "批次未登记监测井"})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].RuleCode == results[j].RuleCode {
			return results[i].SubjectID < results[j].SubjectID
		}
		return results[i].RuleCode < results[j].RuleCode
	})
	b, _ := json.Marshal(results)
	sum := sha256.Sum256(b)
	return QualityCheck{ID: checkID, CampaignID: c.ID, RuleSetVersion: c.RuleSetVersion, FactsRevision: c.FactsRevision, Results: results, ResultDigest: hex.EncodeToString(sum[:]), CheckedAt: now.UTC(), CheckedBy: actor}, nil
}

func choose(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func FailedKey(r QualityResult) string { return fmt.Sprintf("%s:%s", r.RuleCode, r.SubjectID) }
