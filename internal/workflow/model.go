package workflow

import (
	"errors"
	"fmt"
	"strings"
)

// Workflow is a linear tool pipeline declared in a skill's workflow.yaml.
type Workflow struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// Step invokes one registered tool; Approve routes it through HITL.
type Step struct {
	ID      string         `yaml:"id"`
	Tool    string         `yaml:"tool"`
	Approve bool           `yaml:"approve"`
	Args    map[string]any `yaml:"args"`
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
		if seen[s.ID] {
			return fmt.Errorf("workflow: duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
		if strings.TrimSpace(s.Tool) == "" {
			return fmt.Errorf("workflow step %q: tool required", s.ID)
		}
	}
	return nil
}
