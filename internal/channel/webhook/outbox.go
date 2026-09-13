package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rebornace/baize/internal/store"
	runwebhook "github.com/rebornace/baize/internal/webhook"
)

type outboxPayload struct {
	Kind           string        `json:"kind"`
	ConversationID string        `json:"conversation_id"`
	Account        string        `json:"account"`
	Peer           Peer          `json:"peer"`
	Text           string        `json:"text,omitempty"`
	Media          []outboxMedia `json:"media,omitempty"`
	RunID          string        `json:"run_id,omitempty"`
	ContextToken   string        `json:"context_token,omitempty"`
}

type outboxMedia struct {
	Name    string `json:"name"`
	MIME    string `json:"mime"`
	BlobKey string `json:"blob_key"`
}

// SetStore atomically replaces the backing store (Store hot-swap).
func (c *Channel) SetStore(st store.Store) {
	c.outboxMu.Lock()
	c.persist = st
	c.outboxMu.Unlock()
}

func (c *Channel) storeRef() store.Store {
	c.outboxMu.RLock()
	defer c.outboxMu.RUnlock()
	return c.persist
}

// RetryOutbound resets a dead/pending outbound row for this channel and wakes the worker.
func (c *Channel) RetryOutbound(id string) error {
	st := c.storeRef()
	if st == nil {
		return fmt.Errorf("webhook: channel outbox store not configured")
	}
	entry, err := st.GetChannelOutbox(id)
	if err != nil {
		return err
	}
	if entry.Channel != c.cfg.Name {
		return store.ErrChannelOutboxNotFound
	}
	if err := st.ResetChannelOutboxRetry(id); err != nil {
		return err
	}
	c.signalOutboxWake()
	return nil
}

// StartOutboxWorker drains due channel_outbox rows for this instance until ctx is cancelled.
func (c *Channel) StartOutboxWorker(ctx context.Context) {
	if c.outboxWake == nil {
		c.outboxMu.Lock()
		if c.outboxWake == nil {
			c.outboxWake = make(chan struct{}, 1)
		}
		c.outboxMu.Unlock()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.outboxWake:
			c.processOutboxDue(ctx)
		case <-ticker.C:
			c.processOutboxDue(ctx)
		}
	}
}

func (c *Channel) signalOutboxWake() {
	if c.outboxWake == nil {
		return
	}
	select {
	case c.outboxWake <- struct{}{}:
	default:
	}
}

func (c *Channel) enqueue(msg OutboundMessage, kind store.ChannelOutboxKind, blobKeys []string) error {
	st := c.storeRef()
	if st == nil {
		return fmt.Errorf("webhook: channel outbox store not configured")
	}
	seq := c.outboxSeq.Add(1)
	deliveryKey := store.ChannelOutboxDeliveryKey(c.cfg.Name, msg.ConversationID, kind, msg.RunID, seq)

	payload := outboxPayload{
		Kind:           msg.Kind,
		ConversationID: msg.ConversationID,
		Account:        msg.Account,
		Peer:           msg.Peer,
		Text:           msg.Text,
		RunID:          msg.RunID,
		ContextToken:   msg.ContextToken,
	}
	if kind == store.ChannelOutboxKindMedia {
		for i, m := range msg.Media {
			key := ""
			if i < len(blobKeys) {
				key = blobKeys[i]
			}
			payload.Media = append(payload.Media, outboxMedia{
				Name:    m.Name,
				MIME:    m.MIME,
				BlobKey: key,
			})
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	blobKeysJSON := []byte("[]")
	if len(blobKeys) > 0 {
		blobKeysJSON, err = json.Marshal(blobKeys)
		if err != nil {
			return err
		}
	}
	entry := store.ChannelOutboxEntry{
		DeliveryKey:    deliveryKey,
		Channel:        c.cfg.Name,
		Kind:           kind,
		PeerID:         msg.Peer.ID,
		ConversationID: msg.ConversationID,
		Account:        msg.Account,
		RunID:          msg.RunID,
		PayloadJSON:    body,
		BlobKeysJSON:   blobKeysJSON,
		TargetURL:      c.cfg.OutboundURL,
	}
	created, _, err := st.PutChannelOutboxIfAbsent(entry)
	if err != nil {
		return err
	}
	if created {
		c.signalOutboxWake()
	}
	return nil
}

func (c *Channel) processOutboxDue(ctx context.Context) {
	st := c.storeRef()
	if st == nil {
		return
	}
	entries, err := st.ListChannelOutboxDue(time.Now().UTC(), 20)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Channel != c.cfg.Name {
			continue
		}
		c.deliverOutboxOne(ctx, entry)
	}
}

func (c *Channel) deliverOutboxOne(ctx context.Context, entry store.ChannelOutboxEntry) {
	st := c.storeRef()
	if st == nil {
		return
	}
	msg, err := c.buildOutboundFromEntry(ctx, entry)
	if err != nil {
		entry.Attempt++
		entry.Status = store.ChannelOutboxDead
		entry.LastError = err.Error()
		_ = st.UpdateChannelOutbox(entry)
		return
	}
	targetURL := entry.TargetURL
	if targetURL == "" {
		targetURL = c.cfg.OutboundURL
	}
	statusCode, postErr := c.sender.postOnce(ctx, targetURL, msg)
	if postErr == nil && statusCode >= 200 && statusCode < 300 {
		entry.Status = store.ChannelOutboxDelivered
		entry.LastError = ""
		_ = st.UpdateChannelOutbox(entry)
		return
	}
	entry.Attempt++
	entry.LastError = formatOutboundDeliveryError(statusCode, postErr)
	if !runwebhook.Retryable(statusCode, postErr) || entry.Attempt >= entry.MaxAttempts {
		entry.Status = store.ChannelOutboxDead
		_ = st.UpdateChannelOutbox(entry)
		return
	}
	entry.NextRetryAt = time.Now().UTC().Add(runwebhook.Backoff(entry.Attempt))
	_ = st.UpdateChannelOutbox(entry)
}

func (c *Channel) buildOutboundFromEntry(ctx context.Context, entry store.ChannelOutboxEntry) (OutboundMessage, error) {
	var p outboxPayload
	if err := json.Unmarshal(entry.PayloadJSON, &p); err != nil {
		return OutboundMessage{}, err
	}
	msg := OutboundMessage{
		Kind:           p.Kind,
		ConversationID: p.ConversationID,
		Account:        p.Account,
		Peer:           p.Peer,
		Text:           p.Text,
		RunID:          p.RunID,
		ContextToken:   p.ContextToken,
	}
	if len(p.Media) == 0 {
		return msg, nil
	}
	if c.blobs == nil {
		return OutboundMessage{}, fmt.Errorf("webhook: channel outbox blob store not configured")
	}
	for _, m := range p.Media {
		data, err := c.blobs.Get(ctx, m.BlobKey)
		if err != nil {
			return OutboundMessage{}, fmt.Errorf("webhook: load outbox blob %q: %w", m.BlobKey, err)
		}
		msg.Media = append(msg.Media, OutboundMedia{
			Name:          m.Name,
			MIME:          m.MIME,
			ContentBase64: base64.StdEncoding.EncodeToString(data),
		})
	}
	return msg, nil
}

func formatOutboundDeliveryError(statusCode int, err error) string {
	if err != nil {
		return err.Error()
	}
	if statusCode > 0 {
		return fmt.Sprintf("webhook: outbound status %d", statusCode)
	}
	return "webhook: outbound delivery failed"
}

// putMediaBlob stores media bytes and returns the blob key.
func (c *Channel) putMediaBlob(ctx context.Context, filename string, mime string, data []byte) (string, error) {
	if c.blobs == nil {
		return "", fmt.Errorf("webhook: channel outbox blob store not configured")
	}
	key := fmt.Sprintf("channel-outbox/%s/%s/%s", c.cfg.Name, uuid.NewString(), filename)
	if err := c.blobs.Put(ctx, key, data, mime); err != nil {
		return "", err
	}
	return key, nil
}
