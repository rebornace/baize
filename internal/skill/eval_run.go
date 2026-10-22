package skill

import (
	"encoding/json"
	"strings"

	"github.com/rebornace/baize/internal/eval/evidence"
	"github.com/rebornace/baize/internal/store"
)

// ScoreEvalCaseFromRun scores one skill eval case from stored run events.
// ExpectState checks are skipped (cleared before scoring); when the case
// defined expect_state, notes include state_check=skipped.
func ScoreEvalCaseFromRun(c EvalCase, evs []store.Event, runStatus string) evidence.Result {
	evCase := ToEvidenceCase(c)
	hadExpectState := len(c.ExpectState) > 0
	evCase.ExpectState = nil

	timeline := evidence.ToolTimeline(storeEventsToEvidence(normalizeToolResultEvents(evs)))
	res := evidence.Score(evCase, timeline, strings.TrimSpace(runStatus))
	if hadExpectState {
		res = appendStateSkippedNote(res)
	}
	return res
}

func appendStateSkippedNote(res evidence.Result) evidence.Result {
	const note = "state_check=skipped"
	if res.Notes == "" {
		res.Notes = note
		return res
	}
	if strings.Contains(res.Notes, note) {
		return res
	}
	res.Notes = res.Notes + "; " + note
	return res
}

func normalizeToolResultEvents(evs []store.Event) []store.Event {
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
