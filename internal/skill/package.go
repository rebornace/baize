package skill

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rebornace/baize/internal/workflow"
)

type Package struct {
	ID                 string
	Name               string
	Description        string
	DescriptionEN      string // optional English catalog blurb (SKILL.en.md)
	Tools              []string
	Body               string
	BodyEN             string // optional English body (SKILL.en.md)
	Source             string // builtin | user | managed
	Dir                string
	Workflow           *workflow.Workflow // optional pipeline from workflow.yaml
	Managed            bool
	ManagedKind        string
	ManagedConnectorID string
}

// NormalizeLocale maps UI/API locale tags to the skill locale keys we support.
// Unknown or empty values return "" (use the default Chinese/SKILL.md body).
func NormalizeLocale(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	l = strings.ReplaceAll(l, "_", "-")
	switch {
	case l == "en" || strings.HasPrefix(l, "en-"):
		return "en"
	case l == "zh" || l == "zh-cn" || l == "zh-hans" || strings.HasPrefix(l, "zh-"):
		return "zh-CN"
	default:
		return ""
	}
}

// LocalizedBody returns BodyEN when locale is English and BodyEN is set;
// otherwise the default Body (SKILL.md).
func (p Package) LocalizedBody(locale string) string {
	if NormalizeLocale(locale) == "en" && strings.TrimSpace(p.BodyEN) != "" {
		return p.BodyEN
	}
	return p.Body
}

// LocalizedDescription returns DescriptionEN when locale is English and set;
// otherwise Description.
func (p Package) LocalizedDescription(locale string) string {
	if NormalizeLocale(locale) == "en" && strings.TrimSpace(p.DescriptionEN) != "" {
		return p.DescriptionEN
	}
	return p.Description
}

// ApplyEnglishOverlay merges an English SKILL.en.md parse into p.
func (p *Package) ApplyEnglishOverlay(en Package) {
	if p == nil {
		return
	}
	if strings.TrimSpace(en.Body) != "" {
		p.BodyEN = en.Body
	}
	if strings.TrimSpace(en.Description) != "" {
		p.DescriptionEN = en.Description
	}
}

type frontmatter struct {
	Name               string   `yaml:"name"`
	Description        string   `yaml:"description"`
	Tools              []string `yaml:"tools"`
	Managed            bool     `yaml:"managed"`
	ManagedKind        string   `yaml:"managed_kind"`
	ManagedConnectorID string   `yaml:"managed_connector_id"`
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
		Name:               strings.TrimSpace(fm.Name),
		Description:        strings.TrimSpace(fm.Description),
		Tools:              fm.Tools,
		Body:               body,
		Managed:            fm.Managed,
		ManagedKind:        strings.TrimSpace(fm.ManagedKind),
		ManagedConnectorID: strings.TrimSpace(fm.ManagedConnectorID),
	}, nil
}
