package canceledwriteerror

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

func TestCanceledWritePreservesContextError(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	writes := []struct {
		name string
		call func(*store.TxStore) error
	}{
		{name: "campaign", call: func(tx *store.TxStore) error { return tx.InsertCampaign(domain.MonitoringCampaign{ID: "campaign-1"}) }},
		{name: "well", call: func(tx *store.TxStore) error {
			return tx.InsertWell(domain.MonitoringWell{ID: "well-1", CampaignID: "campaign-1", PlannedAnalytes: []string{"pH"}})
		}},
		{name: "sample", call: func(tx *store.TxStore) error {
			return tx.InsertSample(domain.SampleRecord{ID: "sample-1", CampaignID: "campaign-1"})
		}},
		{name: "dataset", call: func(tx *store.TxStore) error {
			return tx.InsertDataset(domain.FrozenDataset{CampaignID: "campaign-1", Content: json.RawMessage(`{}`)})
		}},
		{name: "credential", call: func(tx *store.TxStore) error {
			return tx.InsertCredential(domain.ReleaseCredential{ID: "credential-1", CampaignID: "campaign-1"})
		}},
	}
	masked := make([]string, 0, len(writes))
	for _, write := range writes {
		ctx, cancel := context.WithCancel(context.Background())
		_, err = repo.Execute(ctx, "cancel-"+write.name, "cancel-"+write.name, func(tx *store.TxStore) (json.RawMessage, error) {
			cancel()
			return nil, write.call(tx)
		})
		if !errors.Is(err, context.Canceled) {
			masked = append(masked, write.name)
		}
	}
	if len(masked) != 0 {
		t.Fatalf("TestCanceledWritePreservesContextError: cancellation was replaced for %s", strings.Join(masked, ","))
	}
}
