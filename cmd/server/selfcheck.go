package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type selfcheckEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
type selfcheckMutation struct {
	CampaignID       string `json:"campaignId"`
	ResourceID       string `json:"resourceId"`
	Status           string `json:"status"`
	Version          int64  `json:"version"`
	EvidenceRevision int64  `json:"evidenceRevision"`
}
type selfcheckCheck struct {
	selfcheckMutation
	Exceptions []struct {
		ID       string `json:"id"`
		RuleCode string `json:"ruleCode"`
	} `json:"exceptions"`
	Check struct {
		ResultDigest string `json:"resultDigest"`
	} `json:"check"`
}

func runSelfcheck(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	end := start.Add(24 * time.Hour)
	created, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns", map[string]any{"idempotencyKey": "self-create", "expectedVersion": 0, "actor": "自检现场负责人", "role": "FIELD_LEAD", "campaignCode": "SC-" + time.Now().UTC().Format("20060102150405.000000000"), "samplingWindowStart": start, "samplingWindowEnd": end, "ruleSetVersion": "GW-QC-1"})
	if err != nil {
		return err
	}
	campaignID := created.CampaignID
	well, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/wells", map[string]any{"idempotencyKey": "self-well", "expectedVersion": created.Version, "actor": "自检现场负责人", "role": "FIELD_LEAD", "wellCode": "W-01", "locationLabel": "自检井位", "plannedAnalytes": []string{"pH", "硝酸盐"}, "responsiblePerson": "自检采样员", "plannedSampleAt": start.Add(2 * time.Hour)})
	if err != nil {
		return err
	}
	version := well.Version
	for i, kind := range []string{"NORMAL", "DUPLICATE"} {
		payload := map[string]any{"idempotencyKey": fmt.Sprintf("self-sample-%d", i), "expectedVersion": version, "actor": "自检采样员", "role": "FIELD_LEAD", "wellId": well.ResourceID, "sampleCode": fmt.Sprintf("SC-S-%d-%d", time.Now().UnixNano(), i), "sampleKind": kind, "collectedAt": start.Add(time.Duration(2+i) * time.Hour), "fieldMeasurements": []map[string]any{{"name": "pH", "value": 7.1 + float64(i)/10, "unit": "pH", "measuredAt": start.Add(time.Duration(2+i) * time.Hour)}}, "preservationMethod": "4℃冷藏", "preservationExpiresAt": start.Add(20 * time.Hour), "custodyEvents": []map[string]any{{"sequence": 1, "action": "现场移交", "toPerson": "实验室接收员", "occurredAt": start.Add(time.Duration(3+i) * time.Hour), "condition": "封签完整"}}}
		sample, postErr := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/samples", payload)
		if postErr != nil {
			return postErr
		}
		version = sample.Version
	}
	check, err := postCheck(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/checks", map[string]any{"idempotencyKey": "self-check", "expectedVersion": version, "actor": "自检质量审核员", "role": "QUALITY_REVIEWER"})
	if err != nil {
		return err
	}
	if len(check.Exceptions) != 1 || check.Exceptions[0].RuleCode != "GW-BLANK-001" {
		return fmt.Errorf("自检预期得到一个空白样异常，实际为 %d 个", len(check.Exceptions))
	}
	exceptionID := check.Exceptions[0].ID
	evidence, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/exceptions/"+exceptionID+"/evidence", map[string]any{"idempotencyKey": "self-evidence", "expectedVersion": check.Version, "actor": "自检现场负责人", "role": "FIELD_LEAD", "kind": "EXPLANATION", "content": "现场空白瓶运输破损，已附批次影响评估并确认不影响目标井样。"})
	if err != nil {
		return err
	}
	review, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/exceptions/"+exceptionID+"/review", map[string]any{"idempotencyKey": "self-review", "expectedVersion": evidence.Version, "actor": "自检质量审核员", "role": "QUALITY_REVIEWER", "evidenceRevision": evidence.EvidenceRevision, "decision": "ACCEPTED", "comment": "影响评估充分，接受说明。"})
	if err != nil {
		return err
	}
	approved, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/approve", map[string]any{"idempotencyKey": "self-approve", "expectedVersion": review.Version, "actor": "自检技术批准人", "role": "TECHNICAL_APPROVER", "checkDigest": check.Check.ResultDigest})
	if err != nil {
		return err
	}
	frozen, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/freeze", map[string]any{"idempotencyKey": "self-freeze", "expectedVersion": approved.Version, "actor": "自检技术批准人", "role": "TECHNICAL_APPROVER"})
	if err != nil {
		return err
	}
	released, err := postMutation(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID+"/credentials", map[string]any{"idempotencyKey": "self-credential", "expectedVersion": frozen.Version, "actor": "自检放行员", "role": "RELEASE_OFFICER"})
	if err != nil {
		return err
	}
	if released.Status != "RELEASED" {
		return fmt.Errorf("凭据签发后状态不是 RELEASED")
	}
	var detail struct {
		Campaign struct {
			Status string `json:"status"`
		} `json:"campaign"`
		Verification struct {
			AuditChainValid    bool `json:"auditChainValid"`
			DatasetDigestValid bool `json:"datasetDigestValid"`
			CredentialValid    bool `json:"credentialValid"`
		} `json:"verification"`
	}
	if err = getData(ctx, client, baseURL+"/api/v1/campaigns/"+campaignID, &detail); err != nil {
		return err
	}
	if detail.Campaign.Status != "RELEASED" || !detail.Verification.AuditChainValid || !detail.Verification.DatasetDigestValid || !detail.Verification.CredentialValid {
		return fmt.Errorf("最终详情或完整性校验不符合预期")
	}
	return nil
}

func postMutation(ctx context.Context, client *http.Client, url string, payload any) (selfcheckMutation, error) {
	var value selfcheckMutation
	err := postData(ctx, client, url, payload, &value)
	return value, err
}
func postCheck(ctx context.Context, client *http.Client, url string, payload any) (selfcheckCheck, error) {
	var value selfcheckCheck
	err := postData(ctx, client, url, payload, &value)
	return value, err
}
func postData(ctx context.Context, client *http.Client, url string, payload, dst any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeSelfcheckResponse(response, dst)
}
func getData(ctx context.Context, client *http.Client, url string, dst any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeSelfcheckResponse(response, dst)
}
func decodeSelfcheckResponse(response *http.Response, dst any) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	var envelope selfcheckEnvelope
	if err = json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("解析自检响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if envelope.Error != nil {
			return fmt.Errorf("HTTP %d %s: %s", response.StatusCode, envelope.Error.Code, envelope.Error.Message)
		}
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, string(body))
	}
	return json.Unmarshal(envelope.Data, dst)
}
