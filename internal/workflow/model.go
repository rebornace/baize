package workflow

import (
	"errors"
	"fmt"
	"strings"
)

// Workflow is a linear tool pipeline declared in a skill's workflow.yaml.
type Workflow struct {
	Name  string `yaml:"name" json:"name"`
	Steps []Step `yaml:"steps" json:"steps"`
}

// Step invokes one registered tool; Approve routes it through HITL. When is an
// optional condition; a false result skips the step and emits step_skipped.
// Retry re-invokes a failed step up to N extra times (0-5). HITL pauses and
// human rejections are never retried. TimeoutMS caps one tool invocation.
// ContinueOnError records a tolerated failure and keeps the pipeline going;
// pauses, rejections and cancellation still stop it.
type Step struct {
	ID              string         `yaml:"id" json:"id"`
	Tool            string         `yaml:"tool" json:"tool"`
	Approve         bool           `yaml:"approve,omitempty" json:"approve,omitempty"`
	When            *Condition     `yaml:"when,omitempty" json:"when,omitempty"`
	Args            map[string]any `yaml:"args,omitempty" json:"args,omitempty"`
	Retry           int            `yaml:"retry,omitempty" json:"retry,omitempty"`
	TimeoutMS       int            `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty"`
	ContinueOnError bool           `yaml:"continue_on_error,omitempty" json:"continue_on_error,omitempty"`
}

func (w *Workflow) Validate() error {
	if strings.TrimSpace(w.Name) == "" {
		return errors.New("workflow: name required")
	}
	if len(w.Steps) == 0 {
		return errors.New("workflow: steps required")
	}
	seen := map[string]bool{}
	for i, s := range w.Steps {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("workflow step %d: id required", i+1)
		}
		if strings.Contains(s.ID, ".") {
			return fmt.Errorf("workflow step %d: id %q must not contain '.'", i+1, s.ID)
		}
		if seen[s.ID] {
			return fmt.Errorf("workflow: duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
		if strings.TrimSpace(s.Tool) == "" {
			return fmt.Errorf("workflow step %q: tool required", s.ID)
		}
		if s.Retry < 0 || s.Retry > 5 {
			return fmt.Errorf("workflow step %q: retry must be between 0 and 5", s.ID)
		}
		if s.TimeoutMS < 0 || s.TimeoutMS > 600000 {
			return fmt.Errorf("workflow step %q: timeout_ms must be between 0 and 600000", s.ID)
		}
		if s.When != nil {
			if err := s.When.Validate(); err != nil {
				return fmt.Errorf("workflow step %q: %w", s.ID, err)
			}
		}
	}
	return nil
}
