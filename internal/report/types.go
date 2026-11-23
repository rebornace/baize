package report

const (
	FormatSections = "sections"
	FormatHTML     = "html"

	MaxHTMLBytes = 512 * 1024
)

// PageRequest is the JSON payload for create_analysis_page.
type PageRequest struct {
	Title    string             `json:"title,omitempty"`
	Format   string             `json:"format"`
	Theme    string             `json:"theme,omitempty"`
	Datasets map[string]Dataset `json:"datasets,omitempty"`
	Filters  []Filter           `json:"filters,omitempty"`
	Sections []Section          `json:"sections,omitempty"`
	HTML     string             `json:"html,omitempty"`
}

// Dataset holds tabular data for client-side binding and filters.
type Dataset struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Filter describes a page-level filter control.
type Filter struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Label   string   `json:"label"`
	Dataset string   `json:"dataset"`
	Field   string   `json:"field"`
	Options []string `json:"options,omitempty"`
	Default string   `json:"default,omitempty"`
}

// Section is one content block in a sections-format page.
type Section struct {
	Type      string         `json:"type"`
	Content   string         `json:"content,omitempty"`
	Items     []KPIItem      `json:"items,omitempty"`
	ID        string         `json:"id,omitempty"`
	Title     string         `json:"title,omitempty"`
	Binding   *Binding       `json:"binding,omitempty"`
	Option    map[string]any `json:"option,omitempty"`
	Drilldown *Drilldown     `json:"drilldown,omitempty"`
	Columns   []string       `json:"columns,omitempty"`
	Rows      [][]any        `json:"rows,omitempty"`
	Sections  []Section      `json:"sections,omitempty"`
}

// KPIItem is one metric card in a kpi section.
type KPIItem struct {
	Label   string   `json:"label"`
	Value   any      `json:"value,omitempty"`
	Binding *Binding `json:"binding,omitempty"`
}

// Binding is the shorthand for deriving chart/table data from datasets.
type Binding struct {
	Dataset   string         `json:"dataset"`
	Chart     string         `json:"chart,omitempty"`
	Category  string         `json:"category,omitempty"`
	Value     map[string]any `json:"value,omitempty"`
	Aggregate string         `json:"aggregate,omitempty"`
	Where     map[string]any `json:"where,omitempty"`
	Columns   []string       `json:"columns,omitempty"`
	Field     string         `json:"field,omitempty"`
}

// Drilldown configures chart click-through behavior.
type Drilldown struct {
	On              string            `json:"on"`
	Action          string            `json:"action"`
	FilterID        string            `json:"filter_id,omitempty"`
	ValueFrom       string            `json:"value_from,omitempty"`
	TargetSectionID string            `json:"target_section_id,omitempty"`
	Match           map[string]string `json:"match,omitempty"`
}
