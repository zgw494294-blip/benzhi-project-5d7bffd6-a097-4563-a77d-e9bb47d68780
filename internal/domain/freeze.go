package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type FrozenDataset struct {
	CampaignID     string          `json:"campaignId"`
	DatasetVersion int64           `json:"datasetVersion"`
	Content        json.RawMessage `json:"content"`
	Digest         string          `json:"digest"`
	FrozenAt       time.Time       `json:"frozenAt"`
	FrozenBy       string          `json:"frozenBy"`
}

type frozenContent struct {
	Campaign   MonitoringCampaign `json:"campaign"`
	Wells      []MonitoringWell   `json:"wells"`
	Samples    []SampleRecord     `json:"samples"`
	Check      QualityCheck       `json:"qualityCheck"`
	Exceptions []QualityException `json:"exceptions"`
}

func BuildFrozenDataset(c MonitoringCampaign, wells []MonitoringWell, samples []SampleRecord, check QualityCheck, exceptions []QualityException, version int64, actor string, now time.Time) (FrozenDataset, error) {
	c.SamplingWindowStart = c.SamplingWindowStart.UTC()
	c.SamplingWindowEnd = c.SamplingWindowEnd.UTC()
	c.CreatedAt = c.CreatedAt.UTC()
	if c.ApprovedAt != nil {
		t := c.ApprovedAt.UTC()
		c.ApprovedAt = &t
	}
	for i := range wells {
		wells[i].PlannedSampleAt = wells[i].PlannedSampleAt.UTC()
	}
	for i := range samples {
		samples[i].CollectedAt = samples[i].CollectedAt.UTC()
		samples[i].PreservationExpiresAt = samples[i].PreservationExpiresAt.UTC()
		for j := range samples[i].FieldMeasurements {
			samples[i].FieldMeasurements[j].MeasuredAt = samples[i].FieldMeasurements[j].MeasuredAt.UTC()
		}
		for j := range samples[i].CustodyEvents {
			samples[i].CustodyEvents[j].OccurredAt = samples[i].CustodyEvents[j].OccurredAt.UTC()
		}
	}
	check.CheckedAt = check.CheckedAt.UTC()
	for i := range exceptions {
		for j := range exceptions[i].EvidenceRevisions {
			exceptions[i].EvidenceRevisions[j].SubmittedAt = exceptions[i].EvidenceRevisions[j].SubmittedAt.UTC()
		}
		if exceptions[i].ReviewedAt != nil {
			t := exceptions[i].ReviewedAt.UTC()
			exceptions[i].ReviewedAt = &t
		}
	}
	sort.Slice(wells, func(i, j int) bool { return wells[i].ID < wells[j].ID })
	sort.Slice(samples, func(i, j int) bool { return samples[i].ID < samples[j].ID })
	sort.Slice(exceptions, func(i, j int) bool { return exceptions[i].ID < exceptions[j].ID })
	content, err := json.Marshal(frozenContent{Campaign: c, Wells: wells, Samples: samples, Check: check, Exceptions: exceptions})
	if err != nil {
		return FrozenDataset{}, err
	}
	sum := sha256.Sum256(content)
	return FrozenDataset{CampaignID: c.ID, DatasetVersion: version, Content: content, Digest: hex.EncodeToString(sum[:]), FrozenAt: now.UTC(), FrozenBy: actor}, nil
}

type ReleaseCredential struct {
	ID               string    `json:"id"`
	CampaignID       string    `json:"campaignId"`
	SerialNumber     int64     `json:"serialNumber"`
	DatasetVersion   int64     `json:"datasetVersion"`
	DatasetDigest    string    `json:"datasetDigest"`
	PreviousDigest   string    `json:"previousDigest"`
	CredentialDigest string    `json:"credentialDigest"`
	IssuedAt         time.Time `json:"issuedAt"`
	IssuedBy         string    `json:"issuedBy"`
}
