package skill

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/rebornace/baize/internal/eval/evidence"
)

const evalHarnessEvidence = "evidence-eval"

var allowedEvalProtocols = map[string]struct{}{
	"V0": {},
	"V1": {},
	"V2": {},
}

// EvalDoc is the on-disk EVAL.yaml top-level shape.
type EvalDoc struct {
	Harness string     `yaml:"harness" json:"harness"`
	Cases   []EvalCase `yaml:"cases" json:"cases"`
}

// EvalCase is one evidence-eval case including the run prompt (input).
type EvalCase struct {
	ID                     string            `yaml:"id" json:"id"`
	Protocol               string            `yaml:"protocol" json:"protocol"`
	Input                  string            `yaml:"input" json:"input"`
	RequiredReads          []string          `yaml:"required_reads,omitempty" json:"required_reads,omitempty"`
	Consequential          []string          `yaml:"consequential,omitempty" json:"consequential,omitempty"`
	ExpectConsequentialAny []string          `yaml:"expect_consequential_any,omitempty" json:"expect_consequential_any,omitempty"`
	Chain                  []EvalChainLink   `yaml:"chain,omitempty" json:"chain,omitempty"`
	ForbidTools            []string          `yaml:"forbid_tools,omitempty" json:"forbid_tools,omitempty"`
	ArgContains            []EvalArgContains `yaml:"arg_contains,omitempty" json:"arg_contains,omitempty"`
	ExpectState            []EvalExpectState `yaml:"expect_state,omitempty" json:"expect_state,omitempty"`
	Notes                  string            `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// EvalChainLink is a V2 ordering constraint: before must follow a successful after.
type EvalChainLink struct {
	After  string `yaml:"after" json:"after"`
	Before string `yaml:"before" json:"before"`
}

// EvalArgContains requires a successful tool call whose args contain a substring.
type EvalArgContains struct {
	Tool   string `yaml:"tool" json:"tool"`
	Substr string `yaml:"substr" json:"substr"`
}

// EvalExpectState is a ticket-shaped expectation for trajectory scoring.
type EvalExpectState struct {
	ID            string `yaml:"id,omitempty" json:"id,omitempty"`
	TitleContains string `yaml:"title_contains,omitempty" json:"title_contains,omitempty"`
	Status        string `yaml:"status,omitempty" json:"status,omitempty"`
}

func parseEvalBytes(raw []byte) (EvalDoc, error) {
	var doc EvalDoc
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return EvalDoc{}, err
	}
	if err := ValidateEvalDoc(&doc); err != nil {
		return EvalDoc{}, err
	}
	return doc, nil
}

// ParseEvalYAML decodes and validates EVAL.yaml bytes.
func ParseEvalYAML(raw []byte) (EvalDoc, error) {
	return parseEvalBytes(raw)
}

// ParseEvalJSON decodes and validates a PUT body (JSON; strict unknown-field rejection at all levels).
func ParseEvalJSON(raw []byte) (EvalDoc, error) {
	return parseEvalBytes(raw)
}

// ValidateEvalDoc checks harness, case fields, and unique case ids.
func ValidateEvalDoc(doc *EvalDoc) error {
	if doc == nil {
		return fmt.Errorf("eval doc is nil")
	}
	if doc.Harness != evalHarnessEvidence {
		return fmt.Errorf("harness must be %q", evalHarnessEvidence)
	}
	seen := make(map[string]struct{}, len(doc.Cases))
	for i, c := range doc.Cases {
		if c.ID == "" {
			return fmt.Errorf("cases[%d]: id must be non-empty", i)
		}
		if _, dup := seen[c.ID]; dup {
			return fmt.Errorf("cases[%d]: duplicate id %q", i, c.ID)
		}
		seen[c.ID] = struct{}{}
		if c.Protocol == "" {
			return fmt.Errorf("cases[%d]: protocol must be non-empty", i)
		}
		if _, ok := allowedEvalProtocols[c.Protocol]; !ok {
			return fmt.Errorf("cases[%d]: protocol must be V0, V1, or V2", i)
		}
		if c.Input == "" {
			return fmt.Errorf("cases[%d]: input must be non-empty", i)
		}
	}
	return nil
}

// MarshalEvalYAML encodes an EvalDoc as YAML.
func MarshalEvalYAML(doc EvalDoc) ([]byte, error) {
	return yaml.Marshal(doc)
}

// ToEvidenceCase maps scoring fields to evidence.Case (excludes Input).
func ToEvidenceCase(c EvalCase) evidence.Case {
	chain := make([]evidence.ChainLink, len(c.Chain))
	for i, link := range c.Chain {
		chain[i] = evidence.ChainLink{After: link.After, Before: link.Before}
	}
	argContains := make([]evidence.ArgContains, len(c.ArgContains))
	for i, ac := range c.ArgContains {
		argContains[i] = evidence.ArgContains{Tool: ac.Tool, Substr: ac.Substr}
	}
	expectState := make([]evidence.ExpectTicket, len(c.ExpectState))
	for i, es := range c.ExpectState {
		expectState[i] = evidence.ExpectTicket{
			ID:            es.ID,
			TitleContains: es.TitleContains,
			Status:        es.Status,
		}
	}
	return evidence.Case{
		ID:                     c.ID,
		Protocol:               c.Protocol,
		RequiredReads:          c.RequiredReads,
		Consequential:          c.Consequential,
		ExpectConsequentialAny: c.ExpectConsequentialAny,
		Chain:                  chain,
		ForbidTools:            c.ForbidTools,
		ArgContains:            argContains,
		ExpectState:            expectState,
	}
}
