package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

const evalHarnessEvidence = "evidence-eval"

// skillEvalRunPollTimeout is the max wait per case before scoring with TIMEOUT.
const skillEvalRunPollTimeout = 120 * time.Second

func evalCaseCount(p skill.Package) int {
	if p.Eval == nil {
		return 0
	}
	return len(p.Eval.Cases)
}

func emptyEvalDoc() skill.EvalDoc {
	return skill.EvalDoc{
		Harness: evalHarnessEvidence,
		Cases:   []skill.EvalCase{},
	}
}

func normalizeEvalDoc(doc skill.EvalDoc) skill.EvalDoc {
	if doc.Cases == nil {
		doc.Cases = []skill.EvalCase{}
	}
	return doc
}

func (s *Server) handleGetSkillEval(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	p, ok := s.SkillCatalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "skill not found")
		return
	}
	if p.Eval != nil {
		writeJSON(w, http.StatusOK, normalizeEvalDoc(*p.Eval))
		return
	}
	raw, found, err := readSkillEvalRaw(r.Context(), s, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, emptyEvalDoc())
		return
	}
	doc, err := skill.ParseEvalYAML(raw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_eval", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, normalizeEvalDoc(doc))
}

func readSkillEvalRaw(ctx context.Context, s *Server, p skill.Package) ([]byte, bool, error) {
	switch p.Source {
	case skill.SourceUser, skill.SourceManaged:
		if s.Blobs == nil {
			return nil, false, errors.New("blob store not configured")
		}
		key := blob.SkillObjectKey(p.Source, p.ID, "EVAL.yaml")
		raw, err := s.Blobs.Get(ctx, key)
		if err != nil {
			if errors.Is(err, blob.ErrNotFound) {
				return nil, false, nil
			}
			return nil, false, err
		}
		return raw, true, nil
	default:
		path := filepath.Join(p.Dir, "EVAL.yaml")
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		return raw, true, nil
	}
}

func (s *Server) handlePutSkillEval(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	doc, err := skill.ParseEvalJSON(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.SkillCatalog.PutEval(id, doc); err != nil {
		writeSkillEvalMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, normalizeEvalDoc(doc))
}

func (s *Server) handleDeleteSkillEval(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	if err := s.SkillCatalog.DeleteEval(id); err != nil {
		writeSkillEvalMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type skillEvalRunCaseResult struct {
	ID    string   `json:"id"`
	Pass  bool     `json:"pass"`
	Codes []string `json:"codes"`
	Notes string   `json:"notes"`
	RunID string   `json:"run_id"`
}

func (s *Server) trySkillEvalRunLock(skillID string) bool {
	_, loaded := s.skillEvalRunBusy.LoadOrStore(skillID, struct{}{})
	return !loaded
}

func (s *Server) unlockSkillEvalRun(skillID string) {
	s.skillEvalRunBusy.Delete(skillID)
}

func (s *Server) resolveSkillEvalDoc(ctx context.Context, p skill.Package) (skill.EvalDoc, error) {
	if p.Eval != nil {
		return normalizeEvalDoc(*p.Eval), nil
	}
	raw, found, err := readSkillEvalRaw(ctx, s, p)
	if err != nil {
		return skill.EvalDoc{}, err
	}
	if !found {
		return emptyEvalDoc(), nil
	}
	doc, err := skill.ParseEvalYAML(raw)
	if err != nil {
		return skill.EvalDoc{}, err
	}
	return normalizeEvalDoc(doc), nil
}

func (s *Server) handlePostSkillEvalRun(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	skillID := strings.TrimSpace(r.PathValue("id"))
	if skillID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	p, ok := s.SkillCatalog.Get(skillID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "skill not found")
		return
	}

	var body struct {
		AgentID string   `json:"agent_id"`
		CaseIDs []string `json:"case_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	agentID := strings.TrimSpace(body.AgentID)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	if _, err := s.Store.GetAgent(agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	doc, err := s.resolveSkillEvalDoc(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_eval", err.Error())
		return
	}
	if len(doc.Cases) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "no eval cases")
		return
	}

	cases, err := selectSkillEvalCases(doc.Cases, body.CaseIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if !s.trySkillEvalRunLock(skillID) {
		writeError(w, http.StatusConflict, "eval_run_busy", "eval run already in progress for this skill")
		return
	}
	defer s.unlockSkillEvalRun(skillID)

	results := make([]skillEvalRunCaseResult, 0, len(cases))
	for _, c := range cases {
		res := s.runAndScoreSkillEvalCase(r.Context(), agentID, skillID, c)
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func selectSkillEvalCases(all []skill.EvalCase, ids []string) ([]skill.EvalCase, error) {
	if len(ids) == 0 {
		return all, nil
	}
	byID := make(map[string]skill.EvalCase, len(all))
	for _, c := range all {
		byID[c.ID] = c
	}
	out := make([]skill.EvalCase, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errors.New("case_ids must not contain empty id")
		}
		c, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown eval case id: %s", id)
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Server) runAndScoreSkillEvalCase(ctx context.Context, agentID, skillID string, c skill.EvalCase) skillEvalRunCaseResult {
	fail := func(codes []string, notes string) skillEvalRunCaseResult {
		return skillEvalRunCaseResult{
			ID:    c.ID,
			Pass:  false,
			Codes: codes,
			Notes: notes,
		}
	}

	runRec, err := s.startRun(ctx, startRunInput{
		AgentID:      agentID,
		Input:        c.Input,
		Skills:       []string{skillID},
		PinnedSkills: []string{skillID},
	})
	if err != nil {
		return fail([]string{"RUN_START"}, err.Error())
	}

	final, timedOut := pollSkillEvalRun(ctx, s.Store, runRec.ID, s.skillEvalRunPollDuration())
	scoreStatus := string(final.Status)
	if timedOut {
		scoreStatus = "timeout"
	}

	evs, err := s.Store.ListEvents(runRec.ID)
	if err != nil {
		return skillEvalRunCaseResult{
			ID:    c.ID,
			Pass:  false,
			Codes: []string{"EVENTS"},
			Notes: err.Error(),
			RunID: runRec.ID,
		}
	}
	scored := skill.ScoreEvalCaseFromRun(c, evs, scoreStatus)
	codes := scored.Codes
	if codes == nil {
		codes = []string{}
	}
	return skillEvalRunCaseResult{
		ID:    c.ID,
		Pass:  scored.Pass,
		Codes: codes,
		Notes: scored.Notes,
		RunID: runRec.ID,
	}
}

func (s *Server) skillEvalRunPollDuration() time.Duration {
	if s != nil && s.skillEvalRunPollTimeout > 0 {
		return s.skillEvalRunPollTimeout
	}
	return skillEvalRunPollTimeout
}

func skillEvalRunTerminal(st store.Status) bool {
	switch st {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected, store.StatusWaitingHuman:
		return true
	default:
		return false
	}
}

func pollSkillEvalRun(ctx context.Context, st store.Store, runID string, timeout time.Duration) (store.Run, bool) {
	deadline := time.Now().Add(timeout)
	var last store.Run
	for {
		runRec, err := st.GetRun(runID)
		if err == nil && runRec != nil {
			last = *runRec
			if skillEvalRunTerminal(runRec.Status) {
				return *runRec, false
			}
		}
		if time.Now().After(deadline) {
			return last, true
		}
		select {
		case <-ctx.Done():
			return last, true
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func writeSkillEvalMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skill.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, skill.ErrBuiltin), errors.Is(err, skill.ErrManaged):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
	}
}
