package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type EvidenceKind string

const (
	EvidenceResample    EvidenceKind = "RESAMPLE"
	EvidenceExplanation EvidenceKind = "EXPLANATION"
)

type EvidenceRevision struct {
	Revision           int64          `json:"revision"`
	Kind               EvidenceKind   `json:"kind"`
	Content            string         `json:"content"`
	ContentSummary     string         `json:"contentSummary"`
	SubmittedBy        string         `json:"submittedBy"`
	SubmittedAt        time.Time      `json:"submittedAt"`
	ReferencesRevision int64          `json:"referencesRevision,omitempty"`
	Status             EvidenceStatus `json:"status"`
	WithdrawnBy        string         `json:"withdrawnBy,omitempty"`
	WithdrawnAt        *time.Time     `json:"withdrawnAt,omitempty"`
	ReviewDecision     ReviewDecision `json:"reviewDecision,omitempty"`
	ReviewComment      string         `json:"reviewComment,omitempty"`
	ReviewedBy         string         `json:"reviewedBy,omitempty"`
	ReviewedAt         *time.Time     `json:"reviewedAt,omitempty"`
}

type QualityException struct {
	ID                string             `json:"id"`
	CampaignID        string             `json:"campaignId"`
	CheckID           string             `json:"checkId"`
	RuleCode          string             `json:"ruleCode"`
	SubjectID         string             `json:"subjectId,omitempty"`
	Severity          RuleSeverity       `json:"severity"`
	Description       string             `json:"description"`
	Status            ExceptionStatus    `json:"status"`
	EvidenceRevisions []EvidenceRevision `json:"evidenceRevisions"`
	Current           bool               `json:"current"`
	InvalidatedAt     *time.Time         `json:"invalidatedAt,omitempty"`
	ReviewDecision    ReviewDecision     `json:"reviewDecision,omitempty"`
	ReviewComment     string             `json:"reviewComment,omitempty"`
	ReviewedBy        string             `json:"reviewedBy,omitempty"`
	ReviewedAt        *time.Time         `json:"reviewedAt,omitempty"`
}

func evidenceSummary(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func (e *QualityException) AddEvidence(kind EvidenceKind, content, actor string, references int64, now time.Time) (int64, error) {
	if !e.Current {
		return 0, NewError(ErrState, "历史检查异常只读，不能提交整改证据")
	}
	if e.Status == ExceptionClosed {
		return 0, NewError(ErrState, "已关闭异常不能追加整改证据")
	}
	if kind != EvidenceResample && kind != EvidenceExplanation {
		return 0, FieldError("kind", "整改证据类型无效")
	}
	content, actor = strings.TrimSpace(content), strings.TrimSpace(actor)
	if content == "" || actor == "" {
		return 0, FieldError("content", "整改内容和提交人不能为空")
	}
	if len(e.EvidenceRevisions) > 0 {
		latest := e.EvidenceRevisions[len(e.EvidenceRevisions)-1]
		if latest.Status == EvidencePending {
			return 0, NewError(ErrConflict, "最新整改证据仍待审核或撤回")
		}
		if references != latest.Revision {
			return 0, &DomainError{Code: ErrStaleVersion, Field: "referencesRevision", Message: "新证据必须引用最新已结束 revision"}
		}
	} else if references != 0 {
		return 0, FieldError("referencesRevision", "首份证据不能引用既有 revision")
	}
	revision := int64(len(e.EvidenceRevisions) + 1)
	e.EvidenceRevisions = append(e.EvidenceRevisions, EvidenceRevision{Revision: revision, Kind: kind, Content: content, ContentSummary: evidenceSummary(content), SubmittedBy: actor, SubmittedAt: now.UTC(), ReferencesRevision: references, Status: EvidencePending})
	e.refreshProjection()
	return revision, nil
}

func (e *QualityException) Withdraw(revision int64, actor string, now time.Time) error {
	if !e.Current || len(e.EvidenceRevisions) == 0 {
		return NewError(ErrState, "没有可撤回的当前整改证据")
	}
	latest := &e.EvidenceRevisions[len(e.EvidenceRevisions)-1]
	if revision != latest.Revision {
		return &DomainError{Code: ErrStaleVersion, Field: "expectedEvidenceRevision", Message: "只能撤回最新证据 revision"}
	}
	if latest.Status != EvidencePending {
		return NewError(ErrState, "已审核或已撤回的证据不能撤回")
	}
	if strings.TrimSpace(actor) != latest.SubmittedBy {
		return NewError(ErrForbidden, "只有该证据提交人可以撤回")
	}
	t := now.UTC()
	latest.Status, latest.WithdrawnBy, latest.WithdrawnAt = EvidenceWithdrawn, actor, &t
	e.refreshProjection()
	return nil
}

func (e *QualityException) Review(revision int64, decision ReviewDecision, comment, reviewer string, now time.Time) error {
	if !e.Current || len(e.EvidenceRevisions) == 0 {
		return NewError(ErrState, "异常没有待审核的整改证据")
	}
	latest := &e.EvidenceRevisions[len(e.EvidenceRevisions)-1]
	if revision != latest.Revision {
		return &DomainError{Code: ErrStaleVersion, Field: "evidenceRevision", Message: "只能审核最新证据 revision"}
	}
	if latest.Status != EvidencePending {
		return NewError(ErrState, "目标证据不处于待审核状态")
	}
	if decision != ReviewAccepted && decision != ReviewRejected {
		return FieldError("decision", "审核结论无效")
	}
	if strings.TrimSpace(reviewer) == "" {
		return FieldError("reviewedBy", "审核人不能为空")
	}
	t := now.UTC()
	latest.ReviewDecision, latest.ReviewComment = decision, strings.TrimSpace(comment)
	latest.ReviewedBy, latest.ReviewedAt = strings.TrimSpace(reviewer), &t
	if decision == ReviewAccepted {
		latest.Status = EvidenceAccepted
	} else {
		latest.Status = EvidenceRejected
	}
	e.refreshProjection()
	return nil
}

func (e *QualityException) refreshProjection() {
	e.Status = ExceptionOpen
	e.ReviewDecision, e.ReviewComment, e.ReviewedBy, e.ReviewedAt = "", "", "", nil
	if len(e.EvidenceRevisions) == 0 {
		return
	}
	latest := e.EvidenceRevisions[len(e.EvidenceRevisions)-1]
	e.ReviewDecision, e.ReviewComment, e.ReviewedBy, e.ReviewedAt = latest.ReviewDecision, latest.ReviewComment, latest.ReviewedBy, latest.ReviewedAt
	switch latest.Status {
	case EvidencePending:
		e.Status = ExceptionPending
	case EvidenceAccepted:
		e.Status = ExceptionClosed
	case EvidenceRejected:
		e.Status = ExceptionRejected
	}
}
