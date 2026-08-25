package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"groundwater-release/internal/domain"
)

func testDataset(campaignID string, content string) domain.FrozenDataset {
	sum := sha256.Sum256([]byte(content))
	return domain.FrozenDataset{CampaignID: campaignID, DatasetVersion: 1, Content: []byte(content), Digest: hex.EncodeToString(sum[:])}
}

func TestVerifyCredentialChainReportsFirstBrokenLink(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d1, d2 := testDataset("c1", "one"), testDataset("c2", "two")
	c1, err := IssueCredential("id1", d1, 1, "", "签发员", now)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := IssueCredential("id2", d2, 2, c1.CredentialDigest, "签发员", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	datasets := map[string]domain.FrozenDataset{"c1": d1, "c2": d2}
	valid := VerifyCredentialChain(c2, []domain.ReleaseCredential{c1, c2}, datasets, nil)
	if !valid.Valid || valid.StartSerial != 1 || valid.EndSerial != 2 {
		t.Fatalf("有效全链被误判: %#v", valid)
	}
	c2.PreviousDigest = "broken"
	broken := VerifyCredentialChain(c2, []domain.ReleaseCredential{c1, c2}, datasets, nil)
	if broken.Valid || broken.FirstFailure == nil || broken.FirstFailure.SerialNumber != 2 || broken.FirstFailure.Code != "PREVIOUS_DIGEST_MISMATCH" {
		t.Fatalf("未准确报告首个断链位置: %#v", broken)
	}
}
