package evidence

import (
	"strconv"
	"strings"
)

// ArgContains requires a successful tool call whose args contain a substring.
type ArgContains struct {
	Tool   string `json:"tool"`
	Substr string `json:"substr"`
}

// ExpectTicket is one expected ticket field match in state scoring.
type ExpectTicket struct {
	ID            string `json:"id,omitempty"`
	TitleContains string `json:"title_contains,omitempty"`
	Status        string `json:"status,omitempty"`
}

// StateTicket is one ticket in an injected state snapshot.
type StateTicket struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// StateSnapshot is the ticket state at scoring time (injected, not from cases.json).
type StateSnapshot struct {
	Tickets []StateTicket `json:"tickets"`
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
	ExpectState            []ExpectTicket `json:"expect_state,omitempty"`
	StateSnapshot          *StateSnapshot `json:"-"`
}

// ChainLink is a V2 ordering constraint: before must follow a successful after.
type ChainLink struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

// Result is the deterministic score output (aligned with Score-EvidenceCase).
type Result struct {
	Pass   bool
	Codes  []string
	Notes  string
}

func ticketMatches(exp ExpectTicket, t StateTicket) bool {
	if exp.ID != "" && t.ID != exp.ID {
		return false
	}
	if exp.Status != "" && t.Status != exp.Status {
		return false
	}
	if exp.TitleContains != "" && !strings.Contains(t.Title, exp.TitleContains) {
		return false
	}
	return true
}

func stateMatches(expect []ExpectTicket, snap StateSnapshot) bool {
	for _, exp := range expect {
		found := false
		for _, t := range snap.Tickets {
			if ticketMatches(exp, t) {
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
