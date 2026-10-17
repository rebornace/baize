package skill

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	ProposeName         = "propose_skill"
	MaxProposeBodyRunes = 20000
)

type ProposeDraft struct {
	Name        string
	Description string
	Tools       []string
	Body        string
}

type proposeFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools,omitempty"`
}

func BuildSKILLMD(d ProposeDraft) ([]byte, error) {
	name := strings.TrimSpace(d.Name)
	if err := validateSkillID(name); err != nil {
		return nil, err
	}
	fm := proposeFrontmatter{
		Name:        name,
		Description: strings.TrimSpace(d.Description),
	}
	if len(d.Tools) > 0 {
		fm.Tools = append([]string(nil), d.Tools...)
	}
	yamlPart, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(yamlPart)
	b.WriteString("---\n\n")
	b.WriteString(d.Body)
	return []byte(b.String()), nil
}

func ValidateProposeArgs(args map[string]any, _ time.Time) error {
	name, err := requiredStringArg(args, "name")
	if err != nil {
		return err
	}
	description, err := requiredStringArg(args, "description")
	if err != nil {
		return err
	}
	body, err := requiredStringArg(args, "body")
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(body) > MaxProposeBodyRunes {
		return fmt.Errorf("body exceeds %d runes", MaxProposeBodyRunes)
	}
	var tools []string
	if raw, ok := args["tools"]; ok && raw != nil {
		tools, err = parseToolsArg(raw)
		if err != nil {
			return err
		}
	}
	raw, err := BuildSKILLMD(ProposeDraft{
		Name:        name,
		Description: description,
		Tools:       tools,
		Body:        body,
	})
	if err != nil {
		return err
	}
	if _, err := ParseSKILLMD(raw); err != nil {
		return err
	}
	return nil
}

func requiredStringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s required", key)
	}
	return s, nil
}

func parseToolsArg(v any) ([]string, error) {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...), nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("tools must be a string array")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("tools must be a string array")
	}
}
