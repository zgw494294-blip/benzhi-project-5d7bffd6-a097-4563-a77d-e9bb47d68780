package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"groundwater-release/internal/audit"
	"groundwater-release/internal/store"
)

type Service struct {
	repo        *store.Repository
	now         func() time.Time
	newID       func() string
	detailMu    sync.RWMutex
	detailCache map[string][]byte
}

func New(repo *store.Repository) *Service {
	return &Service{
		repo:        repo,
		now:         time.Now,
		newID:       randomID,
		detailCache: make(map[string][]byte),
	}
}

func (s *Service) cachedDetail(campaignID string) ([]byte, bool) {
	s.detailMu.RLock()
	defer s.detailMu.RUnlock()
	data, ok := s.detailCache[campaignID]
	return append([]byte(nil), data...), ok
}

func (s *Service) rememberDetail(campaignID string, data []byte) {
	s.detailMu.Lock()
	defer s.detailMu.Unlock()
	s.detailCache[campaignID] = append([]byte(nil), data...)
}

// invalidateDetail drops any cached campaign detail so the next query
// rebuilds it from the store. It is called after a mutation commits but
// before the mutation returns to the caller, which keeps the invalidation
// ordered after the commit and prevents a stale pre-write detail from
// surviving the mutation. Failed mutations leave the cache untouched so a
// previously cached valid result remains usable.
func (s *Service) invalidateDetail(campaignID string) {
	s.detailMu.Lock()
	defer s.detailMu.Unlock()
	delete(s.detailCache, campaignID)
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func execute[T any](ctx context.Context, s *Service, campaignID, key, operation string, fn func(*store.TxStore) (T, error)) (T, error) {
	var zero T
	result, err := s.repo.Execute(ctx, key, operation, func(tx *store.TxStore) (json.RawMessage, error) {
		value, err := fn(tx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	})
	if err != nil {
		return zero, err
	}
	if err = json.Unmarshal(result.Response, &zero); err != nil {
		return zero, err
	}
	if !result.Replayed {
		// Drop any cached detail so the next CampaignDetail reads the newly
		// committed state. This runs before the mutation returns to the
		// caller, which orders the invalidation after the commit and keeps a
		// concurrent query that started before the write from overwriting the
		// post-write state with a stale snapshot. Failed mutations leave the
		// cache untouched so a previously cached valid result remains usable.
		s.invalidateDetail(campaignID)
	}
	switch value := any(&zero).(type) {
	case *MutationResult:
		value.Replayed = result.Replayed
	case *BatchWellResult:
		value.Replayed = result.Replayed
	case *CheckResult:
		value.Replayed = result.Replayed
	}
	return zero, nil
}

func appendAudit(tx *store.TxStore, campaignID, eventType, actor string, payload any, now time.Time) error {
	events, err := tx.ListAuditEvents(campaignID)
	if err != nil {
		return err
	}
	event, err := audit.AppendEvent(events, campaignID, eventType, actor, payload, now)
	if err != nil {
		return err
	}
	return tx.InsertAuditEvent(event)
}

func mutation(campaignID, resourceID string, status string, version int64) MutationResult {
	return MutationResult{CampaignID: campaignID, ResourceID: resourceID, Status: statusToDomain(status), Version: version}
}
