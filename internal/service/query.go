package service

import (
	"context"
	"sync"

	"groundwater-release/internal/audit"
	"groundwater-release/internal/domain"
	"groundwater-release/internal/store"
)

type CampaignDetail struct {
	store.CampaignSnapshot
	PendingExceptions int                `json:"pendingExceptions"`
	Verification      audit.Verification `json:"verification"`
}

type verificationCall struct {
	done   chan struct{}
	report audit.FullChainVerification
	err    error

	mu         sync.Mutex
	waiters    int
	workCancel context.CancelFunc
}

// register records a new waiter for this shared verification. It returns true
// when the waiter has been admitted, or false when the work already finished
// (in which case the caller should read call.report/call.err directly).
func (c *verificationCall) register() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return false
	default:
	}
	c.waiters++
	return true
}

// leave decrements the waiter count and, when no waiter remains interested,
// cancels the shared background context so the verification worker stops
// holding database resources.
func (c *verificationCall) leave() {
	c.mu.Lock()
	if c.waiters > 0 {
		c.waiters--
	}
	if c.waiters == 0 {
		if c.workCancel != nil {
			c.workCancel()
		}
	}
	c.mu.Unlock()
}

func (s *Service) CampaignDetail(ctx context.Context, campaignID string) (CampaignDetail, error) {
	snapshot, err := s.repo.Snapshot(ctx, campaignID)
	if err != nil {
		return CampaignDetail{}, err
	}
	pending := 0
	for _, e := range snapshot.Exceptions {
		if e.Current && e.Status != domain.ExceptionClosed {
			pending++
		}
	}
	var previous *domain.ReleaseCredential
	if snapshot.Credential != nil && snapshot.Credential.SerialNumber > 1 {
		err = s.repo.View(ctx, func(tx *store.TxStore) error {
			var inner error
			previous, inner = tx.PreviousCredential(snapshot.Credential.SerialNumber)
			return inner
		})
		if err != nil {
			return CampaignDetail{}, err
		}
	}
	verification := audit.Verify(snapshot, previous)
	if snapshot.Credential != nil {
		full, fullErr := s.CredentialVerification(ctx, campaignID)
		if fullErr != nil {
			return CampaignDetail{}, fullErr
		}
		verification.AuditChainValid = full.Items.AuditChain
		verification.DatasetDigestValid = full.Items.DatasetDigests
		verification.CredentialValid = full.Items.SequenceChain && full.Items.CredentialDigests
		if !full.Valid && full.FirstFailure != nil {
			verification.Message = full.FirstFailure.Message
		} else {
			verification.Message = "完整性校验通过"
		}
	}
	return CampaignDetail{CampaignSnapshot: snapshot, PendingExceptions: pending, Verification: verification}, nil
}

func (s *Service) CredentialVerification(ctx context.Context, campaignID string) (audit.FullChainVerification, error) {
	s.verificationMu.Lock()
	if call, ok := s.verificationCalls[campaignID]; ok {
		s.verificationMu.Unlock()
		if !call.register() {
			return call.report, call.err
		}
		defer call.leave()
		select {
		case <-ctx.Done():
			return audit.FullChainVerification{}, ctx.Err()
		case <-call.done:
			return call.report, call.err
		}
	}
	workCtx, workCancel := context.WithCancel(context.Background())
	call := &verificationCall{done: make(chan struct{}), workCancel: workCancel}
	s.verificationCalls[campaignID] = call
	s.verificationMu.Unlock()

	call.register()
	defer call.leave()
	go s.runCredentialVerification(call, workCtx, campaignID)

	select {
	case <-ctx.Done():
		return audit.FullChainVerification{}, ctx.Err()
	case <-call.done:
		return call.report, call.err
	}
}

func (s *Service) runCredentialVerification(call *verificationCall, workCtx context.Context, campaignID string) {
	report, err := s.loadCredentialVerification(workCtx, campaignID)
	call.report, call.err = report, err
	s.verificationMu.Lock()
	delete(s.verificationCalls, campaignID)
	s.verificationMu.Unlock()
	close(call.done)
}

func (s *Service) loadCredentialVerification(ctx context.Context, campaignID string) (audit.FullChainVerification, error) {
	var report audit.FullChainVerification
	err := s.repo.View(ctx, func(tx *store.TxStore) error {
		if _, err := tx.LoadCampaign(campaignID); err != nil {
			return err
		}
		target, err := tx.LoadCredential(campaignID)
		if err != nil {
			return err
		}
		if target == nil {
			return domain.NewError(domain.ErrNotFound, "该批次尚未签发放行凭据")
		}
		credentials, err := tx.ListCredentialsThrough(target.SerialNumber)
		if err != nil {
			return err
		}
		datasets, err := tx.LoadDatasetsForCredentials(credentials)
		if err != nil {
			return err
		}
		events, err := tx.ListAuditEvents(campaignID)
		if err != nil {
			return err
		}
		report = audit.VerifyCredentialChain(*target, credentials, datasets, events)
		return nil
	})
	return report, err
}
