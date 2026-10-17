package eval

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rebornace/baize/internal/eval/evidence"
	"github.com/rebornace/baize/internal/store"
)

// IngestInput carries run linkage, optional replace, and evidence expectations.
type IngestInput struct {
	RunID           string
	Replace         bool
	Protocol        string
	RequiredReads   []string
	Consequential   []string
	ExpectConseqAny []string
	Chain           []store.EvalChainLink
	ForbidTools     []string
	ArgContains     []store.EvalArgContains
	ExpectState     []store.EvalExpectState
}

// PatchInput updates expectations and optionally promotes draft to confirmed.
type PatchInput struct {
	Protocol        string
	SetProtocol     bool
	RequiredReads   []string
	SetRequiredReads bool
	Consequential   []string
	SetConsequential bool
	ExpectConseqAny []string
	SetExpectConseqAny bool
	Chain           []store.EvalChainLink
	SetChain        bool
	ForbidTools     []string
	SetForbidTools  bool
	ArgContains     []store.EvalArgContains
	SetArgContains  bool
	ExpectState     []store.EvalExpectState
	SetExpectState  bool
	Status          store.EvalCaseStatus
	SetStatus       bool
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

func cloneChain(in []store.EvalChainLink) []store.EvalChainLink {
	if in == nil {
		return nil
	}
	return append([]store.EvalChainLink(nil), in...)
}

func cloneArgContains(in []store.EvalArgContains) []store.EvalArgContains {
	if in == nil {
		return nil
	}
	return append([]store.EvalArgContains(nil), in...)
}

func cloneExpectState(in []store.EvalExpectState) []store.EvalExpectState {
	if in == nil {
		return nil
	}
	return append([]store.EvalExpectState(nil), in...)
}

func validProtocol(p string) bool {
	switch strings.TrimSpace(p) {
	case "V0", "V1", "V2":
		return true
	default:
		return false
	}
}

// expectationsComplete reports whether protocol and required expectation slices were all provided.
// Empty slices count as provided; nil means missing (e.g. JSON field omitted).
func expectationsComplete(c store.EvalCase) bool {
	return expectationsCompleteFields(c.Protocol, c.RequiredReads, c.Consequential)
}

func expectationsCompleteIngest(in IngestInput) bool {
	return expectationsCompleteFields(in.Protocol, in.RequiredReads, in.Consequential)
}

func expectationsCompleteFields(protocol string, requiredReads, consequential []string) bool {
	if !validProtocol(protocol) {
		return false
	}
	if requiredReads == nil || consequential == nil {
		return false
	}
	return true
}

func statusFromExpectations(in IngestInput) store.EvalCaseStatus {
	if expectationsCompleteIngest(in) {
		return store.EvalCaseConfirmed
	}
	return store.EvalCaseDraft
}

// IngestFromRun snapshots run events into a new or replaced EvalCase.
func IngestFromRun(st store.Store, in IngestInput) (store.EvalCase, error) {
	run, err := st.GetRun(in.RunID)
	if err != nil {
		return store.EvalCase{}, ErrRunNotFound
	}
	existing, found, err := st.FindEvalCaseBySourceRun(in.RunID)
	if err != nil {
		return store.EvalCase{}, err
	}
	if found && !in.Replace {
		return store.EvalCase{}, ErrConflict
	}

	evs, err := st.ListEvents(in.RunID)
	if err != nil {
		return store.EvalCase{}, err
	}
	raw, err := json.Marshal(evs)
	if err != nil {
		return store.EvalCase{}, err
	}

	now := time.Now().UTC()
	c := store.EvalCase{
		SourceRunID:       in.RunID,
		Status:            statusFromExpectations(in),
		Protocol:          strings.TrimSpace(in.Protocol),
		Input:             run.Input,
		RequiredReads:     cloneStringSlice(in.RequiredReads),
		Consequential:     cloneStringSlice(in.Consequential),
		ExpectConseqAny:   cloneStringSlice(in.ExpectConseqAny),
		Chain:             cloneChain(in.Chain),
		ForbidTools:       cloneStringSlice(in.ForbidTools),
		ArgContains:       cloneArgContains(in.ArgContains),
		ExpectState:       cloneExpectState(in.ExpectState),
		EventsJSON:        raw,
		RunStatusAtIngest: string(run.Status),
		UpdatedAt:         now,
	}
	if found && in.Replace {
		c.ID = existing.ID
		c.CreatedAt = existing.CreatedAt
	} else {
		c.ID = "ec_" + uuid.NewString()
		c.CreatedAt = now
	}
	if err := st.UpsertEvalCase(c); err != nil {
		return store.EvalCase{}, err
	}
	return st.GetEvalCase(c.ID)
}

// PatchCase merges expectation updates and can set status when expectations are complete.
func PatchCase(st store.Store, id string, patch PatchInput) (store.EvalCase, error) {
	c, err := st.GetEvalCase(id)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		return store.EvalCase{}, ErrCaseNotFound
	}
	if err != nil {
		return store.EvalCase{}, err
	}
	if patch.SetProtocol {
		c.Protocol = strings.TrimSpace(patch.Protocol)
	}
	if patch.SetRequiredReads {
		c.RequiredReads = cloneStringSlice(patch.RequiredReads)
	}
	if patch.SetConsequential {
		c.Consequential = cloneStringSlice(patch.Consequential)
	}
	if patch.SetExpectConseqAny {
		c.ExpectConseqAny = cloneStringSlice(patch.ExpectConseqAny)
	}
	if patch.SetChain {
		c.Chain = cloneChain(patch.Chain)
	}
	if patch.SetForbidTools {
		c.ForbidTools = cloneStringSlice(patch.ForbidTools)
	}
	if patch.SetArgContains {
		c.ArgContains = cloneArgContains(patch.ArgContains)
	}
	if patch.SetExpectState {
		c.ExpectState = cloneExpectState(patch.ExpectState)
	}
	if patch.SetStatus {
		if patch.Status == store.EvalCaseConfirmed && !expectationsComplete(c) {
			return store.EvalCase{}, ErrNotScorable
		}
		c.Status = patch.Status
	} else if c.Status == store.EvalCaseDraft && expectationsComplete(c) {
		c.Status = store.EvalCaseConfirmed
	}
	c.UpdatedAt = time.Now().UTC()
	if err := st.UpsertEvalCase(c); err != nil {
		return store.EvalCase{}, err
	}
	return st.GetEvalCase(c.ID)
}

// normalizeExportEvents copies tool_call_id into id on tool.result when id is missing,
// so exported snapshots match evidence-eval / score.ps1 expectations.
func normalizeExportEvents(evs []store.Event) []store.Event {
	out := make([]store.Event, len(evs))
	copy(out, evs)
	for i := range out {
		if out[i].Type != "tool.result" || out[i].Data == nil {
			continue
		}
		data := out[i].Data
		id, _ := data["id"].(string)
		if id != "" {
			continue
		}
		tcid, _ := data["tool_call_id"].(string)
		if tcid == "" {
			continue
		}
		cloned := make(map[string]any, len(data)+1)
		for k, v := range data {
			cloned[k] = v
		}
		cloned["id"] = tcid
		out[i].Data = cloned
	}
	return out
}

func storeEventsToEvidence(evs []store.Event) []evidence.Event {
	out := make([]evidence.Event, 0, len(evs))
	for _, ev := range evs {
		data, err := json.Marshal(ev.Data)
		if err != nil {
			continue
		}
		out = append(out, evidence.Event{Type: ev.Type, Data: data})
	}
	return out
}

func evidenceCaseFromStore(c store.EvalCase) evidence.Case {
	chain := make([]evidence.ChainLink, 0, len(c.Chain))
	for _, link := range c.Chain {
		chain = append(chain, evidence.ChainLink{After: link.After, Before: link.Before})
	}
	argContains := make([]evidence.ArgContains, 0, len(c.ArgContains))
	for _, ac := range c.ArgContains {
		argContains = append(argContains, evidence.ArgContains{Tool: ac.Tool, Substr: ac.Substr})
	}
	expectState := make([]evidence.ExpectTicket, 0, len(c.ExpectState))
	for _, es := range c.ExpectState {
		expectState = append(expectState, evidence.ExpectTicket{
			ID:            es.ID,
			TitleContains: es.TitleContains,
			Status:        es.Status,
		})
	}
	out := evidence.Case{
		ID:                     c.ID,
		Protocol:               c.Protocol,
		RequiredReads:          c.RequiredReads,
		Consequential:          c.Consequential,
		ExpectConsequentialAny: c.ExpectConseqAny,
		Chain:                  chain,
		ForbidTools:            cloneStringSlice(c.ForbidTools),
		ArgContains:            argContains,
		ExpectState:            expectState,
	}
	if len(c.StateSnapshotJSON) > 0 {
		var snap evidence.StateSnapshot
		if err := json.Unmarshal(c.StateSnapshotJSON, &snap); err == nil {
			out.StateSnapshot = &snap
		}
	}
	return out
}

func scoreEvalCase(c store.EvalCase) (evidence.Result, error) {
	if c.Status != store.EvalCaseConfirmed || !expectationsComplete(c) {
		return evidence.Result{}, ErrNotScorable
	}
	var evs []store.Event
	if err := json.Unmarshal(c.EventsJSON, &evs); err != nil {
		return evidence.Result{}, err
	}
	return evidence.Score(
		evidenceCaseFromStore(c),
		evidence.ToolTimeline(storeEventsToEvidence(evs)),
		c.RunStatusAtIngest,
	), nil
}

func applyScoreResult(c store.EvalCase, res evidence.Result, now time.Time) store.EvalCase {
	pass := res.Pass
	c.LastScorePass = &pass
	c.LastScoreCodes = cloneStringSlice(res.Codes)
	c.LastScoreNotes = res.Notes
	c.LastScoredAt = &now
	c.UpdatedAt = now
	return c
}

// ScoreCase runs deterministic evidence scoring for a confirmed case.
func ScoreCase(st store.Store, caseID string) (evidence.Result, error) {
	c, err := st.GetEvalCase(caseID)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		return evidence.Result{}, ErrCaseNotFound
	}
	if err != nil {
		return evidence.Result{}, err
	}
	res, err := scoreEvalCase(c)
	if err != nil {
		return evidence.Result{}, err
	}
	now := time.Now().UTC()
	c = applyScoreResult(c, res, now)
	if err := st.UpsertEvalCase(c); err != nil {
		return evidence.Result{}, err
	}
	return res, nil
}

type exportCase struct {
	ID                string                  `json:"id"`
	Protocol          string                  `json:"protocol"`
	Input             string                  `json:"input"`
	RequiredReads     []string                `json:"required_reads"`
	Consequential     []string                `json:"consequential"`
	ExpectConseqAny   []string                `json:"expect_consequential_any,omitempty"`
	Chain             []store.EvalChainLink   `json:"chain,omitempty"`
	ForbidTools       []string                `json:"forbid_tools,omitempty"`
	ArgContains       []store.EvalArgContains `json:"arg_contains,omitempty"`
	ExpectState       []store.EvalExpectState `json:"expect_state,omitempty"`
	StateSnapshot     *evidence.StateSnapshot `json:"state_snapshot,omitempty"`
	SourceRunID       string                  `json:"source_run_id"`
	RunStatusAtIngest string                  `json:"run_status_at_ingest"`
}

type exportPayload struct {
	Case   exportCase    `json:"case"`
	Events []store.Event `json:"events"`
}

// Export returns evidence-eval compatible JSON for one case.
func Export(st store.Store, caseID string) ([]byte, error) {
	c, err := st.GetEvalCase(caseID)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		return nil, ErrCaseNotFound
	}
	if err != nil {
		return nil, err
	}
	var evs []store.Event
	if err := json.Unmarshal(c.EventsJSON, &evs); err != nil {
		return nil, err
	}
	exp := exportCase{
		ID:                c.ID,
		Protocol:          c.Protocol,
		Input:             c.Input,
		RequiredReads:     c.RequiredReads,
		Consequential:     c.Consequential,
		ExpectConseqAny:   c.ExpectConseqAny,
		Chain:             c.Chain,
		ForbidTools:       c.ForbidTools,
		ArgContains:       c.ArgContains,
		ExpectState:       c.ExpectState,
		SourceRunID:       c.SourceRunID,
		RunStatusAtIngest: c.RunStatusAtIngest,
	}
	if len(c.StateSnapshotJSON) > 0 {
		var snap evidence.StateSnapshot
		if err := json.Unmarshal(c.StateSnapshotJSON, &snap); err == nil {
			exp.StateSnapshot = &snap
		}
	}
	payload := exportPayload{
		Case:   exp,
		Events: normalizeExportEvents(evs),
	}
	return json.Marshal(payload)
}
