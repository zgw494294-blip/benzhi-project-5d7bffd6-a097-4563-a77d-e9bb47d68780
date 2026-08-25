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
	repo            *store.Repository
	now             func() time.Time
	newID           func() string
	replayMu        sync.Mutex
	replayResponses map[string]json.RawMessage
}

func New(repo *store.Repository) *Service {
	return &Service{repo: repo, now: time.Now, newID: randomID, replayResponses: map[string]json.RawMessage{}}
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func execute[T any](ctx context.Context, s *Service, key, operation string, fn func(*store.TxStore) (T, error)) (T, error) {
	var zero T
	if response, ok := s.cachedReplay(key); ok {
		if err := json.Unmarshal(response, &zero); err != nil {
			return zero, err
		}
		markReplayed(&zero)
		return zero, nil
	}
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
	s.rememberReplay(key, result.Response)
	if result.Replayed {
		markReplayed(&zero)
	}
	return zero, nil
}

func (s *Service) cachedReplay(key string) (json.RawMessage, bool) {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	response, ok := s.replayResponses[key]
	return append(json.RawMessage(nil), response...), ok
}

func (s *Service) rememberReplay(key string, response json.RawMessage) {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	s.replayResponses[key] = append(json.RawMessage(nil), response...)
}

func markReplayed[T any](value *T) {
	switch result := any(value).(type) {
	case *MutationResult:
		result.Replayed = true
	case *BatchWellResult:
		result.Replayed = true
	case *CheckResult:
		result.Replayed = true
	}
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
