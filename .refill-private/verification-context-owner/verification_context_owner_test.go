package verification_context_owner_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"groundwater-release/internal/audit"
	"groundwater-release/internal/domain"
	"groundwater-release/internal/httpapi"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

type doneObservedContext struct {
	context.Context
	once     sync.Once
	observed chan struct{}
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func seedCredential(t *testing.T, repo *store.Repository) string {
	t.Helper()
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	campaign, err := domain.NewCampaign("campaign-context-owner", "CTX-OWNER-1", now.Add(-time.Hour), now.Add(time.Hour), "GW-QC-1", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	content := json.RawMessage(`{"campaignId":"campaign-context-owner","version":1}`)
	sum := sha256.Sum256(content)
	dataset := domain.FrozenDataset{
		CampaignID:     campaign.ID,
		DatasetVersion: 1,
		Content:        content,
		Digest:         hex.EncodeToString(sum[:]),
		FrozenAt:       now,
		FrozenBy:       "技术批准人",
	}
	credential, err := audit.IssueCredential("credential-context-owner", dataset, 1, "", "放行员", now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Execute(context.Background(), "seed-context-owner", "seed-context-owner", func(tx *store.TxStore) (json.RawMessage, error) {
		if err := tx.InsertCampaign(campaign); err != nil {
			return nil, err
		}
		if err := tx.InsertDataset(dataset); err != nil {
			return nil, err
		}
		if err := tx.InsertCredential(credential); err != nil {
			return nil, err
		}
		return json.RawMessage(`{}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return campaign.ID
}

func TestCredentialVerificationKeepsLiveWaiterContext(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "context-owner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	campaignID := seedCredential(t, repo)
	handler := httpapi.New(service.New(repo)).Handler()

	holderStarted := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderResult := make(chan error, 1)
	go func() {
		_, holdErr := repo.Execute(context.Background(), "hold-context-owner", "hold-context-owner", func(_ *store.TxStore) (json.RawMessage, error) {
			close(holderStarted)
			<-releaseHolder
			return json.RawMessage(`{}`), nil
		})
		holderResult <- holdErr
	}()
	<-holderStarted

	firstBase, cancelFirst := context.WithCancel(context.Background())
	firstWaiting := make(chan struct{})
	firstCtx := &doneObservedContext{Context: firstBase, observed: firstWaiting}
	firstResult := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+campaignID+"/credentials/verification", nil).WithContext(firstCtx)
		handler.ServeHTTP(recorder, request)
		firstResult <- recorder.Code
	}()
	<-firstWaiting

	secondAttached := make(chan struct{})
	secondCtx := &doneObservedContext{Context: context.Background(), observed: secondAttached}
	secondResult := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+campaignID+"/credentials/verification", nil).WithContext(secondCtx)
		handler.ServeHTTP(recorder, request)
		secondResult <- recorder.Code
	}()
	<-secondAttached

	cancelFirst()
	firstStatus := <-firstResult
	close(releaseHolder)
	secondStatus := <-secondResult
	holdErr := <-holderResult
	if holdErr != nil {
		t.Fatalf("释放 SQLite 连接失败: %v", holdErr)
	}
	if firstStatus != http.StatusInternalServerError {
		t.Fatalf("被取消的首位调用者应失败，实际 HTTP 状态为 %d", firstStatus)
	}
	if secondStatus != http.StatusOK {
		t.Fatalf("仍存活的复用调用者继承了首位调用者的取消，实际 HTTP 状态为 %d", secondStatus)
	}
}
