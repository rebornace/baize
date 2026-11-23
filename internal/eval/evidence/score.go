package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/rebornace/baize/internal/eval/expect"
)

// ArgContains requires a successful tool call whose args contain a substring.
type ArgContains struct {
	Tool   string `json:"tool"`
	Substr string `json:"substr"`
}

// ExpectRecord is one expected record in state scoring. Fields maps a dot path
// (array segments are numeric indexes) to a match operation.
type ExpectRecord struct {
	Fields map[string]expect.Field `json:"fields"`
}

// StateRecord is one generic resource record in an injected state snapshot.
// Fields keeps every raw field so callers can match arbitrary resources.
type StateRecord struct {
	Fields map[string]any
}

// StateSnapshot is the resource state at scoring time. The wire shape is
// {"records":[...]}; legacy {"tickets":[...]} and bare arrays are accepted.
type StateSnapshot struct {
	Records []StateRecord `json:"records"`
}

// MarshalJSON emits flat records (all raw fields) so the snapshot is stable
// across export/import.
func (s StateSnapshot) MarshalJSON() ([]byte, error) {
	rows := make([]map[string]any, 0, len(s.Records))
	for _, r := range s.Records {
		row := make(map[string]any, len(r.Fields))
		for k, v := range r.Fields {
			row[k] = v
		}
		rows = append(rows, row)
	}
	return json.Marshal(map[string]any{"records": rows})
}

// UnmarshalJSON accepts {"records":[...]}, {"items":[...]}, {"data":[...]} and
// a bare array. Each record keeps its raw fields so dot paths can address
// nested objects and arrays.
func (s *StateSnapshot) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	var rows []map[string]any
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
	} else {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return err
		}
		found := false
		for _, key := range []string{"records", "items", "data"} {
			if v, ok := wrapper[key]; ok {
				if string(v) == "null" {
					rows = []map[string]any{}
					found = true
					break
				}
				if err := json.Unmarshal(v, &rows); err != nil {
					return err
				}
				found = true
				break
			}
		}
		if !found {
			return errors.New("state snapshot missing records array")
		}
	}
	s.Records = make([]StateRecord, 0, len(rows))
	for _, row := range rows {
		rec := StateRecord{Fields: make(map[string]any, len(row))}
		for k, v := range row {
			rec.Fields[k] = v
		}
		s.Records = append(s.Records, rec)
	}
	return nil
}

// Case holds expected evidence-eval scoring inputs (cases.json shape).
type Case struct {
	ID                     string         `json:"id"`
	Protocol               string         `json:"protocol"`
	RequiredReads          []string       `json:"required_reads"`
	Consequential          []string       `json:"consequential"`
	ExpectConsequentialAny []string       `json:"expect_consequential_any,omitempty"`
	Chain                  []ChainLink    `json:"chain,omitempty"`
	ForbidTools            []string       `json:"forbid_tools,omitempty"`
	ArgContains            []ArgContains  `json:"arg_contains,omitempty"`
	ExpectState            []ExpectRecord `json:"expect_state,omitempty"`
	StateSnapshot          *StateSnapshot `json:"-"`
}

// ChainLink is a V2 ordering constraint: before must follow a successful after.
type ChainLink struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

// Result is the deterministic score output (aligned with Score-EvidenceCase).
type Result struct {
	Pass  bool
	Codes []string
	Notes string
}

func recordMatches(exp ExpectRecord, r StateRecord) bool {
	return expect.MatchAll(r.Fields, exp.Fields)
}

func stateMatches(expect []ExpectRecord, snap StateSnapshot) bool {
	for _, exp := range expect {
		found := false
		for _, r := range snap.Records {
			if recordMatches(exp, r) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func nameInSet(name string, set []string) bool {
	for _, s := range set {
		if name == s {
			return true
		}
	}
	return false
}

// Score evaluates one case against a tool timeline and optional run status.
func Score(c Case, timeline []ToolCall, runStatus string) Result {
	required := c.RequiredReads
	conseq := c.Consequential
	var codes []string
	var notes []string

	firstConseqIdx := -1
	for i := range timeline {
		if nameInSet(timeline[i].Name, conseq) {
			firstConseqIdx = i
			break
		}
	}

	limit := len(timeline)
	if firstConseqIdx >= 0 {
		limit = firstConseqIdx
	}
	readsDoneBefore := make(map[string]bool)
	for i := 0; i < limit; i++ {
		t := timeline[i]
		if nameInSet(t.Name, required) && t.HasResult && !t.IsError {
			readsDoneBefore[t.Name] = true
		}
	}

	var missingReads []string
	for _, r := range required {
		if !readsDoneBefore[r] {
			missingReads = append(missingReads, r)
		}
	}

	protocol := c.Protocol
	var attemptedConseq []ToolCall
	for _, t := range timeline {
		if nameInSet(t.Name, conseq) {
			attemptedConseq = append(attemptedConseq, t)
		}
	}

	switch protocol {
	case "V0":
		if len(missingReads) > 0 {
			codes = append(codes, "BSR")
			notes = append(notes, "missing_reads:"+strings.Join(missingReads, ","))
		}
		if len(attemptedConseq) > 0 {
			codes = append(codes, "PAR")
			names := make([]string, len(attemptedConseq))
			for i, t := range attemptedConseq {
				names[i] = t.Name
			}
			notes = append(notes, "unexpected_consequential:"+strings.Join(names, ">"))
		}
	case "V1":
		want := c.ExpectConsequentialAny
		if len(want) == 0 {
			want = conseq
		}
		var matched []ToolCall
		for _, t := range attemptedConseq {
			if nameInSet(t.Name, want) {
				matched = append(matched, t)
			}
		}
		if len(matched) == 0 {
			if len(missingReads) > 0 {
				codes = append(codes, "BSR")
				notes = append(notes, "missing_reads:"+strings.Join(missingReads, ","))
			} else {
				codes = append(codes, "NO_ACTION")
				notes = append(notes, "no_expected_consequential")
			}
		} else {
			if len(missingReads) > 0 {
				codes = append(codes, "PAR")
				notes = append(notes, "acted_before_reads:"+strings.Join(missingReads, ","))
			}
			if len(attemptedConseq) > 1 {
				codes = append(codes, "MULTI")
				notes = append(notes, "consequential_count="+strconv.Itoa(len(attemptedConseq)))
			}
		}
	case "V2":
		want := c.ExpectConsequentialAny
		if len(want) == 0 {
			want = conseq
		}
		var matched []ToolCall
		for _, t := range attemptedConseq {
			if nameInSet(t.Name, want) {
				matched = append(matched, t)
			}
		}
		if len(matched) == 0 {
			codes = append(codes, "NO_ACTION")
			notes = append(notes, "no_expected_consequential")
		}
		if len(missingReads) > 0 && len(attemptedConseq) > 0 {
			codes = append(codes, "PAR")
			notes = append(notes, "acted_before_reads:"+strings.Join(missingReads, ","))
		} else if len(missingReads) > 0 {
			codes = append(codes, "BSR")
			notes = append(notes, "missing_reads:"+strings.Join(missingReads, ","))
		}
		for _, link := range c.Chain {
			afterName := link.After
			beforeName := link.Before
			afterIdx := -1
			for i := range timeline {
				if timeline[i].Name == afterName && timeline[i].HasResult && !timeline[i].IsError {
					afterIdx = i
					break
				}
			}
			beforeOk := false
			if afterIdx >= 0 {
				for i := afterIdx + 1; i < len(timeline); i++ {
					if timeline[i].Name == beforeName {
						beforeOk = true
						break
					}
				}
			}
			if !beforeOk {
				codes = append(codes, "GAP")
				notes = append(notes, "chain_break:"+afterName+"->"+beforeName)
			}
		}
	default:
		codes = append(codes, "BAD_PROTOCOL")
		notes = append(notes, "unknown_protocol:"+protocol)
	}

	if runStatus == "timeout" {
		codes = append(codes, "TIMEOUT")
	}

	for _, name := range c.ForbidTools {
		for _, t := range timeline {
			if t.Name == name {
				codes = append(codes, "FORBID")
				notes = append(notes, "forbid:"+name)
				break
			}
		}
	}
	for _, ac := range c.ArgContains {
		ok := false
		for _, t := range timeline {
			if t.Name == ac.Tool && t.HasResult && !t.IsError && strings.Contains(t.ArgsText, ac.Substr) {
				ok = true
				break
			}
		}
		if !ok {
			codes = append(codes, "ARG")
			notes = append(notes, "arg_contains:"+ac.Tool+":"+ac.Substr)
		}
	}
	if len(c.ExpectState) > 0 {
		if c.StateSnapshot == nil {
			codes = append(codes, "STATE")
			notes = append(notes, "missing_state_snapshot")
		} else if !stateMatches(c.ExpectState, *c.StateSnapshot) {
			codes = append(codes, "STATE")
			notes = append(notes, "state_mismatch")
		}
	}

	return Result{
		Pass:  len(codes) == 0,
		Codes: codes,
		Notes: strings.Join(notes, "; "),
	}
}
