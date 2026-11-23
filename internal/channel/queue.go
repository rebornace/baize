package channel

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rebornace/baize/internal/store"
)

const (
	// QueuedReply acknowledges the first text message received while the
	// conversation is busy. Further messages in the same batch are silent.
	QueuedReply = "收到，忙完这轮后一起处理"

	// maxQueuedRunes caps the merged input per conversation. Longer batches
	// are truncated with queueTruncatedNote so a spammer cannot grow memory
	// without bound.
	maxQueuedRunes     = 4000
	queueTruncatedNote = "\n…（消息过长，后续已截断）"
	queuePollInterval  = 2 * time.Second
	queueSendTimeout   = 10 * time.Second
	queueStartTimeout  = 60 * time.Second
	// maxPendingAttachmentBytes caps the bytes carried per conversation batch.
	maxPendingAttachmentBytes = 10 << 20
)

var errQueueFull = errors.New("channel: queued attachments exceed the size limit")

// PendingStore is the optional persistence capability used to survive a
// restart. Narrow test run stores may omit it; the channel then stays
// memory-only, exactly like v0.
type PendingStore interface {
	UpsertChannelPending(p store.ChannelPending) error
	ListChannelPending(source string) ([]store.ChannelPending, error)
	DeleteChannelPending(conversationID, batchID string) error
}

// pendingInbound is one merged batch for a conversation.
type pendingInbound struct {
	batchID   string
	peerID    string
	ch        Channel
	extras    map[string]string
	text      string
	files     []InboundFile
	fileBytes int
	truncated bool
}

// appendText adds one inbound text to the batch, preserving arrival order and
// enforcing maxQueuedRunes.
func (p *pendingInbound) appendText(text string) {
	text = strings.TrimSpace(text)
	if text == "" || p.truncated {
		return
	}
	if p.text == "" {
		p.text = text
	} else {
		p.text += "\n\n" + text
	}
	if utf8.RuneCountInString(p.text) <= maxQueuedRunes {
		return
	}
	p.text = truncateRunes(p.text, maxQueuedRunes) + queueTruncatedNote
	p.truncated = true
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	out := make([]rune, 0, limit)
	for _, r := range s {
		if len(out) == limit {
			break
		}
		out = append(out, r)
	}
	return string(out)
}

// hasQueued reports whether a batch is waiting or currently being started.
// Callers use it to append instead of starting a competing run.
func (r *Runtime) hasQueued(convID string) bool {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	_, pending := r.pending[convID]
	return pending || r.claiming[convID]
}

// queueInbound appends text to the conversation's batch and makes sure one
// sweeper is running. The first message of a batch gets an ack; a failed ack
// is logged but never drops the message or fails the handler (webhook retries
// would otherwise duplicate it).
func (r *Runtime) queueInbound(ch Channel, convID, peerID string, extras map[string]string, text string, files []InboundFile, quiet bool) error {
	text = strings.TrimSpace(text)
	if text == "" && len(files) == 0 {
		return nil
	}
	var sendAck bool
	r.queueMu.Lock()
	fileBytes := 0
	if cur := r.pending[convID]; cur != nil {
		fileBytes = cur.fileBytes
	}
	for _, f := range files {
		fileBytes += len(f.Data)
	}
	if len(files) > 0 && fileBytes > r.attachmentLimit() {
		r.queueMu.Unlock()
		return errQueueFull
	}
	if r.pending == nil {
		r.pending = map[string]*pendingInbound{}
	}
	p := r.pending[convID]
	if p == nil {
		p = &pendingInbound{batchID: uuid.NewString(), peerID: peerID, ch: ch, extras: extras}
		r.pending[convID] = p
		sendAck = !quiet
	} else {
		// Keep the newest routing context; text order is preserved.
		p.peerID = peerID
		p.ch = ch
		if len(extras) > 0 {
			p.extras = extras
		}
	}
	p.appendText(text)
	if len(files) > 0 {
		p.files = append(p.files, files...)
		p.fileBytes = fileBytes
	}
	r.persistPendingLocked(convID, p)
	if !r.sweeping {
		r.sweeping = true
		go r.sweepLoop()
	}
	r.queueMu.Unlock()

	if sendAck {
		ctx, cancel := context.WithTimeout(context.Background(), queueSendTimeout)
		defer cancel()
		if err := ch.SendText(ctx, peerID, QueuedReply, extras); err != nil {
			log.Printf("channel: queued ack: %v", err)
		}
	}
	return nil
}

// sweepLoop starts idle batches one at a time. It exits when no batch is
// pending; the next inbound message starts a fresh sweeper.
func (r *Runtime) sweepLoop() {
	for {
		r.queueMu.Lock()
		if len(r.pending) == 0 {
			r.sweeping = false
			r.queueMu.Unlock()
			return
		}
		ids := make([]string, 0, len(r.pending))
		for id := range r.pending {
			ids = append(ids, id)
		}
		r.queueMu.Unlock()

		for _, convID := range ids {
			if r.isClaiming(convID) {
				continue
			}
			busy, err := r.Runs.HasActiveRun(convID)
			if err != nil {
				log.Printf("channel: queued drain check %s: %v", convID, err)
				continue
			}
			if busy {
				continue
			}
			p := r.claimPending(convID)
			if p == nil {
				continue
			}
			err = r.runPending(convID, p)
			r.releaseClaim(convID)
			if err != nil {
				log.Printf("channel: queued drain %s: %v", convID, err)
				r.restorePending(convID, p)
				r.retireSweeper()
				return
			}
			// The run is created; drop the persisted batch by id so a newer
			// batch for the same conversation (queued during the start) stays.
			if ps := r.pendingStore(); ps != nil && p.batchID != "" {
				if derr := ps.DeleteChannelPending(convID, p.batchID); derr != nil {
					log.Printf("channel: delete persisted pending %s: %v", convID, derr)
				}
			}
		}
		r.waitSweep()
	}
}

func (r *Runtime) runPending(convID string, p *pendingInbound) error {
	ctx, cancel := context.WithTimeout(context.Background(), queueStartTimeout)
	defer cancel()
	return r.handleInbound(ctx, p.ch, Inbound{
		PeerID: p.peerID,
		Text:   p.text,
		Files:  p.files,
		Extras: p.extras,
	}, true)
}

func (r *Runtime) isClaiming(convID string) bool {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	return r.claiming[convID]
}

func (r *Runtime) claimPending(convID string) *pendingInbound {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	p := r.pending[convID]
	if p == nil {
		return nil
	}
	delete(r.pending, convID)
	if r.claiming == nil {
		r.claiming = map[string]bool{}
	}
	r.claiming[convID] = true
	return p
}

func (r *Runtime) releaseClaim(convID string) {
	r.queueMu.Lock()
	delete(r.claiming, convID)
	r.queueMu.Unlock()
}

func (r *Runtime) restorePending(convID string, p *pendingInbound) {
	if p == nil || strings.TrimSpace(p.text) == "" {
		return
	}
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	if r.pending == nil {
		r.pending = map[string]*pendingInbound{}
	}
	if cur := r.pending[convID]; cur != nil {
		if p.text != "" && cur.text != "" {
			p.text += "\n\n" + cur.text
		} else {
			p.text += cur.text
		}
		p.truncated = p.truncated || cur.truncated
	}
	r.pending[convID] = p
}

// retireSweeper stops the current sweeper. Pending batches stay in memory and
// the next inbound message re-arms a sweeper. Used only when starting a batch
// failed (store/meta error); retrying forever would spam a permanent failure.
func (r *Runtime) retireSweeper() {
	r.queueMu.Lock()
	r.sweeping = false
	r.queueMu.Unlock()
}

func (r *Runtime) waitSweep() {
	if r.drainTick != nil {
		<-r.drainTick
		return
	}
	time.Sleep(queuePollInterval)
}

// source returns the conversation id prefix for this runtime.
func (r *Runtime) source() string {
	if s := strings.TrimSpace(r.Source); s != "" {
		return s
	}
	return "weixin"
}

func (r *Runtime) pendingStore() PendingStore {
	if r == nil || r.Runs == nil {
		return nil
	}
	ps, _ := r.Runs.(PendingStore)
	return ps
}

func (r *Runtime) attachmentLimit() int {
	if r.maxPendingAttachmentBytes > 0 {
		return r.maxPendingAttachmentBytes
	}
	return maxPendingAttachmentBytes
}

// persistPendingLocked upserts the batch; callers must hold queueMu so two
// appends cannot write an older snapshot over a newer one. Best-effort: a
// failed write leaves the in-memory batch working and only logs.
func (r *Runtime) persistPendingLocked(convID string, p *pendingInbound) {
	ps := r.pendingStore()
	if ps == nil || p == nil || p.batchID == "" {
		return
	}
	attachments := make([]store.PendingAttachment, 0, len(p.files))
	for _, f := range p.files {
		attachments = append(attachments, store.PendingAttachment{
			Name: f.Name,
			MIME: f.MIME,
			Data: f.Data,
		})
	}
	err := ps.UpsertChannelPending(store.ChannelPending{
		ConversationID: convID,
		BatchID:        p.batchID,
		Source:         r.source(),
		Account:        AccountFromConvID(convID),
		PeerID:         p.peerID,
		Text:           p.text,
		Attachments:    attachments,
		Truncated:      p.truncated,
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		log.Printf("channel: persist pending %s: %v", convID, err)
	}
}

func pendingFilesFromAttachments(atts []store.PendingAttachment) ([]InboundFile, int) {
	if len(atts) == 0 {
		return nil, 0
	}
	files := make([]InboundFile, 0, len(atts))
	total := 0
	for _, a := range atts {
		files = append(files, InboundFile{Name: a.Name, MIME: a.MIME, Data: a.Data})
		total += len(a.Data)
	}
	return files, total
}

// RestorePending reloads text batches persisted before a restart. Bootstrap
// supplies the channel instance; rows are filtered by this runtime's source.
// Messages are merged oldest-first; if a crash left both a claimed batch and a
// newer one, both are kept (at-least-once, never loss).
func (r *Runtime) RestorePending(ch Channel) {
	if r == nil || ch == nil {
		return
	}
	ps := r.pendingStore()
	if ps == nil {
		return
	}
	rows, err := ps.ListChannelPending(r.source())
	if err != nil {
		log.Printf("channel: restore pending: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	r.queueMu.Lock()
	if r.pending == nil {
		r.pending = map[string]*pendingInbound{}
	}
	restored := 0
	var mergedExtras []store.ChannelPending
	for _, row := range rows {
		convID := strings.TrimSpace(row.ConversationID)
		text := strings.TrimSpace(row.Text)
		if convID == "" || text == "" {
			continue
		}
		files, fileBytes := pendingFilesFromAttachments(row.Attachments)
		p := r.pending[convID]
		if p == nil {
			r.pending[convID] = &pendingInbound{
				batchID:   row.BatchID,
				peerID:    row.PeerID,
				ch:        ch,
				extras:    map[string]string{"account": row.Account},
				text:      text,
				files:     files,
				fileBytes: fileBytes,
				truncated: row.Truncated,
			}
			restored++
			continue
		}
		// More than one row for a conversation is a crash artifact: a batch
		// was claimed, its run was created, but the delete did not land.
		if p.text != "" {
			p.text += "\n\n" + text
		} else {
			p.text = text
		}
		p.files = append(p.files, files...)
		p.fileBytes += fileBytes
		p.truncated = p.truncated || row.Truncated
		mergedExtras = append(mergedExtras, row)
	}
	// Persist the merged text under the kept batch id first; only then drop the
	// extra rows. A crash in between duplicates text instead of losing it.
	for convID, p := range r.pending {
		r.persistPendingLocked(convID, p)
	}
	for _, row := range mergedExtras {
		if err := ps.DeleteChannelPending(row.ConversationID, row.BatchID); err != nil {
			log.Printf("channel: drop merged pending row %s: %v", row.ConversationID, err)
		}
	}
	if len(r.pending) > 0 && !r.sweeping {
		r.sweeping = true
		go r.sweepLoop()
	}
	r.queueMu.Unlock()
	log.Printf("channel: restored %d busy-merge conversation(s) for source %q", restored, r.source())
}
