package workflow

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Marshal encodes a workflow as workflow.yaml.
func Marshal(w *Workflow) ([]byte, error) {
	return yaml.Marshal(w)
}

// ValidateReferences checks that template and condition paths start with
// "input" or a step id declared before the current step. It is intentionally
// structural: input key existence is a runtime/domain concern.
func ValidateReferences(w *Workflow) error {
	if w == nil {
		return nil
	}
	allowed := map[string]bool{"input": true}
	for _, step := range w.Steps {
		if step.When != nil {
			if err := validateConditionRefs(step.When, allowed); err != nil {
				return fmt.Errorf("step %q condition: %w", step.ID, err)
			}
		}
		refs := map[string]bool{}
		collectTemplateRefs(step.Args, refs)
		for path := range refs {
			root := path
			if i := strings.IndexByte(root, '.'); i >= 0 {
				root = root[:i]
			}
			if !allowed[root] {
				return fmt.Errorf("step %q: reference %q must start with input or a previous step id", step.ID, path)
			}
		}
		allowed[step.ID] = true
	}
	return nil
}

func validateConditionRefs(c *Condition, allowed map[string]bool) error {
	if c == nil {
		return nil
	}
	for i := range c.All {
		if err := validateConditionRefs(&c.All[i], allowed); err != nil {
			return err
		}
	}
	for i := range c.Any {
		if err := validateConditionRefs(&c.Any[i], allowed); err != nil {
			return err
		}
	}
	if c.Not != nil {
		return validateConditionRefs(c.Not, allowed)
	}
	root := c.Path
	if i := strings.IndexByte(root, '.'); i >= 0 {
		root = root[:i]
	}
	if root != "" && !allowed[root] {
		return fmt.Errorf("path %q must start with input or a previous step id", c.Path)
	}
	return nil
}

func collectTemplateRefs(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for _, vv := range x {
			collectTemplateRefs(vv, out)
		}
	case []any:
		for _, vv := range x {
			collectTemplateRefs(vv, out)
		}
	case string:
		for _, m := range phRe.FindAllStringSubmatch(x, -1) {
			if len(m) > 1 {
				out[m[1]] = true
			}
		}
	}
}
