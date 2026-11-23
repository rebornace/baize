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

// parseStateResponse accepts a bare array or {"records"|"items"|"data":
// [...]} envelopes.
func parseStateResponse(body []byte) (evidence.StateSnapshot, error) {
	var snap evidence.StateSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return evidence.StateSnapshot{}, fmt.Errorf("invalid state json: %w", err)
	}
	if snap.Records == nil {
		return evidence.StateSnapshot{}, errors.New("state response missing records array")
	}
	return snap, nil
}

// CheckState fetches a resource-state endpoint, scores in memory, and persists
// the snapshot + score in one Upsert. stateURL is the full endpoint URL.
func CheckState(st store.Store, caseID string, stateURL string, client *http.Client) (store.EvalCase, evidence.Result, error) {
	stateURL = strings.TrimSpace(stateURL)
	if stateURL == "" {
		return store.EvalCase{}, evidence.Result{}, ErrStateURLRequired
	}
	c, err := st.GetEvalCase(caseID)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		return store.EvalCase{}, evidence.Result{}, ErrCaseNotFound
	}
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}

	req, err := http.NewRequest(http.MethodGet, stateURL, nil)
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
		return store.EvalCase{}, evidence.Result{}, fmt.Errorf("state endpoint http %d", resp.StatusCode)
	}

	snap, err := parseStateResponse(body)
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
	_ = st.AppendEvalScore(store.EvalScoreRecord{
		CaseID: c.ID, Pass: res.Pass, Codes: cloneStringSlice(res.Codes), Notes: res.Notes, ScoredAt: now,
	})
	updated, err := st.GetEvalCase(caseID)
	if err != nil {
		return store.EvalCase{}, evidence.Result{}, err
	}
	return updated, res, nil
}
