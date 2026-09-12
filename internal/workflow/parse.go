package workflow

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Parse decodes workflow.yaml strictly: unknown fields are rejected outright.
func Parse(raw []byte) (*Workflow, error) {
	var w Workflow
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // 未知字段直接报错 —— 锁死语法面
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("workflow.yaml: %w", err)
	}
	if err := (&w).Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}
