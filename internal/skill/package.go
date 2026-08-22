package skill

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type Package struct {
	ID          string
	Name        string
	Description string
	Tools       []string
	Body        string
	Source      string // builtin | user
	Dir         string
}

type frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
}

func ParseSKILLMD(raw []byte) (Package, error) {
	const delim = "---"
	s := string(raw)
	if !strings.HasPrefix(strings.TrimSpace(s), delim) {
		return Package{}, fmt.Errorf("missing frontmatter")
	}
	rest := strings.TrimSpace(s)
	rest = strings.TrimPrefix(rest, delim)
	end := strings.Index(rest, "\n"+delim)
	if end < 0 {
		return Package{}, fmt.Errorf("unclosed frontmatter")
	}
	yamlPart := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n"+delim):])
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return Package{}, fmt.Errorf("frontmatter: %w", err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return Package{}, fmt.Errorf("name required")
	}
	return Package{
		Name:        strings.TrimSpace(fm.Name),
		Description: strings.TrimSpace(fm.Description),
		Tools:       fm.Tools,
		Body:        body,
	}, nil
}
