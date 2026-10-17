package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/eval/evidence"
	"github.com/rebornace/baize/internal/store"
)

const checkStateHTTPTimeout = 10 * time.Second

func checkStateHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: checkStateHTTPTimeout}
}

func parseTicketsResponse(body []byte) (evidence.StateSnapshot, error) {
	var tickets []evidence.StateTicket
	if err := json.Unmarshal(body, &tickets); err == nil {
		return evidence.StateSnapshot{Tickets: tickets}, nil
	}
	var wrapper struct {
		Tickets []evidence.StateTicket `json:"tickets"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return evidence.StateSnapshot{}, fmt.Errorf("invalid tickets json: %w", err)
	}
	if wrapper.Tickets == nil {
		return evidence.StateSnapshot{}, errors.New("tickets response missing tickets array")
	}
	return evidence.StateSnapshot{Tickets: wrapper.Tickets}, nil
}

// CheckState fetches mock ticket state, scores in memory, and persists snapshot + score in one Upsert.
func CheckState(st store.Store, caseID string, mockBaseURL string, client *http.Client) (store.EvalCase, evidence.Result, error) {
	if strings.TrimSpace(mockBaseURL) == "" {
		return store.EvalCase{}, evidence.Result{}, ErrMockBaseRequired
	}
	c, err := st.GetEvalCase(caseID)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		return store.EvalCase{}, evidence.Result{}, ErrCaseNotFound
	}
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}

	base := strings.TrimRight(strings.TrimSpace(mockBaseURL), "/")
	req, err := http.NewRequest(http.MethodGet, base+"/tickets", nil)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	resp, err := checkStateHTTPClient(client).Do(req)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return store.EvalCase{}, evidence.Result{}, fmt.Errorf("mock tickets http %d", resp.StatusCode)
	}

	snap, err := parseTicketsResponse(body)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}

	c.StateSnapshotJSON = raw
	res, err := scoreEvalCase(c)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	now := time.Now().UTC()
	c = applyScoreResult(c, res, now)
	if err := st.UpsertEvalCase(c); err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	updated, err := st.GetEvalCase(caseID)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	return updated, res, nil
}
