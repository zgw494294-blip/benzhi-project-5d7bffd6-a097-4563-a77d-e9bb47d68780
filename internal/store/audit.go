package store

import (
	"encoding/json"
	"time"
)

type AuditEvent struct {
	CampaignID     string          `json:"campaignId"`
	Sequence       int64           `json:"sequence"`
	EventType      string          `json:"eventType"`
	Actor          string          `json:"actor"`
	OccurredAt     time.Time       `json:"occurredAt"`
	Payload        json.RawMessage `json:"payload"`
	PreviousDigest string          `json:"previousDigest"`
	Digest         string          `json:"digest"`
}

func (s *TxStore) ListAuditEvents(campaignID string) ([]AuditEvent, error) {
	rows, err := s.tx.QueryContext(s.ctx, `SELECT campaign_id,sequence,event_type,actor,occurred_at,payload,previous_digest,digest FROM audit_events WHERE campaign_id=? ORDER BY sequence`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var occurred string
		if err = rows.Scan(&e.CampaignID, &e.Sequence, &e.EventType, &e.Actor, &occurred, &e.Payload, &e.PreviousDigest, &e.Digest); err != nil {
			return nil, err
		}
		if e.OccurredAt, err = parseTime(occurred); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

func (s *TxStore) InsertAuditEvent(e AuditEvent) error {
	_, err := s.tx.ExecContext(s.ctx, `INSERT INTO audit_events(campaign_id,sequence,event_type,actor,occurred_at,payload,previous_digest,digest) VALUES(?,?,?,?,?,?,?,?)`, e.CampaignID, e.Sequence, e.EventType, e.Actor, formatTime(e.OccurredAt), e.Payload, e.PreviousDigest, e.Digest)
	return err
}
