package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/store"
)

const (
	inboundJobPollInterval = 5 * time.Second
	inboundJobTimeout      = 2 * time.Minute
	inboundJobDoneTTL      = 10 * time.Minute
)

func (c *Channel) wakeInboundWorker() {
	if c.inboundWake == nil {
		return
	}
	select {
	case c.inboundWake <- struct{}{}:
	default:
	}
}

// runInboundWorker drains persisted inbound jobs at startup and after each
// wake/poll. Single worker per process: multiple replicas would need a claim
// lease, which is out of scope (channels are documented as single-replica).
func (c *Channel) runInboundWorker(ctx context.Context) {
	if c.persist == nil {
		return
	}
	ticker := time.NewTicker(inboundJobPollInterval)
	defer ticker.Stop()
	for {
		c.processInboundJobs(ctx)
		select {
		case <-ctx.Done():
			return
		case <-c.inboundWake:
		case <-ticker.C:
		}
	}
}

func (c *Channel) processInboundJobs(ctx context.Context) {
	if c.persist == nil {
		return
	}
	_ = c.persist.DeleteWebhookInboundJobsBefore(time.Now().UTC().Add(-inboundJobDoneTTL))
	jobs, err := c.persist.ListWebhookInboundJobsDue(time.Now().UTC(), 20)
	if err != nil {
		log.Printf("webhook %s: list inbound jobs: %v", c.cfg.Name, err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		c.processInboundJob(job)
	}
}

func (c *Channel) processInboundJob(job store.WebhookInboundJob) {
	if c.rt == nil {
		c.failInboundJob(job, errors.New("runtime not ready"))
		return
	}
	var msg InboundMessage
	if err := json.Unmarshal(job.Body, &msg); err != nil {
		c.deadInboundJob(job, err)
		return
	}
	if msg.Event != "" && msg.Event != "message" {
		c.finishInboundJob(job)
		return
	}
	peer := strings.TrimSpace(msg.Peer.ID)
	if peer == "" {
		c.deadInboundJob(job, errors.New("peer.id required"))
		return
	}
	if !c.peerAllowed(peer) {
		log.Printf("webhook %s: peer %q not in allowlist; dropped", c.cfg.Name, peer)
		c.finishInboundJob(job)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), inboundJobTimeout)
	defer cancel()
	fetch := c.fetch
	if fetch == nil {
		fetch = newFetchClient()
	}
	files, err := c.resolveAttachments(ctx, fetch, msg.Attachments)
	if err != nil {
		c.failInboundJob(job, err)
		return
	}
	acct := strings.TrimSpace(msg.Account)
	if acct == "" {
		acct = c.cfg.Account
	}
	c.setActiveAccount(acct)
	extras := map[string]string{"account": acct}
	if msg.ContextToken != "" {
		extras["context_token"] = msg.ContextToken
	}
	in := channel.Inbound{PeerID: peer, Text: msg.Text, Files: files, Extras: extras}
	if err := c.rt.HandleInbound(ctx, c, in); err != nil {
		c.failInboundJob(job, err)
		return
	}
	c.finishInboundJob(job)
}

func (c *Channel) finishInboundJob(job store.WebhookInboundJob) {
	if c.persist == nil {
		return
	}
	job.Status = store.WebhookInboundDone
	job.LastError = ""
	if err := c.persist.UpdateWebhookInboundJob(job); err != nil {
		log.Printf("webhook %s: mark inbound job done: %v", c.cfg.Name, err)
	}
}

func (c *Channel) deadInboundJob(job store.WebhookInboundJob, cause error) {
	if c.persist == nil {
		return
	}
	job.Status = store.WebhookInboundDead
	job.LastError = cause.Error()
	if err := c.persist.UpdateWebhookInboundJob(job); err != nil {
		log.Printf("webhook %s: mark inbound job dead: %v", c.cfg.Name, err)
	}
}

func (c *Channel) failInboundJob(job store.WebhookInboundJob, cause error) {
	if c.persist == nil {
		return
	}
	job.Attempt++
	job.LastError = cause.Error()
	if job.MaxAttempts > 0 && job.Attempt >= job.MaxAttempts {
		job.Status = store.WebhookInboundDead
	} else {
		job.Status = store.WebhookInboundPending
		delay := time.Duration(job.Attempt) * 5 * time.Second
		if delay > time.Minute {
			delay = time.Minute
		}
		job.NextAttemptAt = time.Now().UTC().Add(delay)
	}
	if err := c.persist.UpdateWebhookInboundJob(job); err != nil {
		log.Printf("webhook %s: update inbound job: %v", c.cfg.Name, err)
	}
}
