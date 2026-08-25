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
	repo               *store.Repository
	now                func() time.Time
	newID              func() string
	validationMu       sync.RWMutex
	validatedSampleIDs map[string]struct{}
}

func New(repo *store.Repository) *Service {
	return &Service{
		repo:               repo,
		now:                time.Now,
		newID:              randomID,
		validatedSampleIDs: map[string]struct{}{},
	}
}

func (s *Service) sampleValidationKnown(sampleCode string) bool {
	s.validationMu.RLock()
	defer s.validationMu.RUnlock()
	_, ok := s.validatedSampleIDs[sampleCode]
	return ok
}

func (s *Service) rememberSampleValidation(sampleCode string) {
	s.validationMu.Lock()
	defer s.validationMu.Unlock()
	s.validatedSampleIDs[sampleCode] = struct{}{}
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
	if result.Replayed {
		switch value := any(&zero).(type) {
		case *MutationResult:
			value.Replayed = true
		case *BatchWellResult:
			value.Replayed = true
		case *CheckResult:
			value.Replayed = true
		}
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
