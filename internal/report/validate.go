package report

import (
	"fmt"
	"strings"
)

// Validate checks a PageRequest against the analysis page schema constraints.
func Validate(req *PageRequest) error {
	if req == nil {
		return fmt.Errorf("page request is required")
	}

	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = FormatSections
	}

	hasSectionsPayload := len(req.Datasets) > 0 || len(req.Filters) > 0 || len(req.Sections) > 0
	hasHTML := strings.TrimSpace(req.HTML) != ""

	switch format {
	case FormatHTML:
		if hasSectionsPayload {
			return fmt.Errorf("format html cannot be combined with datasets, filters, or sections")
		}
		return validateHTML(req.HTML)
	case FormatSections:
		if hasHTML {
			return fmt.Errorf("format sections cannot be combined with html")
		}
		if err := validateDatasets(req.Datasets); err != nil {
			return err
		}
		if err := validateFilters(req.Filters, req.Datasets); err != nil {
			return err
		}
		return validateSections(req.Sections, req.Datasets)
	default:
		return fmt.Errorf("unknown format %q", req.Format)
	}
}

func validateHTML(html string) error {
	if len(html) > MaxHTMLBytes {
		return fmt.Errorf("html exceeds maximum size of %d bytes", MaxHTMLBytes)
	}
	if strings.Contains(strings.ToLower(html), `<script src="https://`) {
		return fmt.Errorf("html must not load external scripts via https src")
	}
	return nil
}

func validateDatasets(datasets map[string]Dataset) error {
	for name, ds := range datasets {
		colCount := len(ds.Columns)
		for i, row := range ds.Rows {
			if len(row) != colCount {
				return fmt.Errorf("dataset %q row %d has %d values, expected %d columns", name, i, len(row), colCount)
			}
		}
	}
	return nil
}

func validateFilters(filters []Filter, datasets map[string]Dataset) error {
	for _, f := range filters {
		if f.Dataset == "" {
			continue
		}
		if _, ok := datasets[f.Dataset]; !ok {
			return fmt.Errorf("filter %q references unknown dataset %q", f.ID, f.Dataset)
		}
	}
	return nil
}

func validateSections(sections []Section, datasets map[string]Dataset) error {
	for i, sec := range sections {
		if err := validateSection(sec, datasets, fmt.Sprintf("sections[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateSection(sec Section, datasets map[string]Dataset, path string) error {
	switch sec.Type {
	case "markdown", "kpi", "table", "echarts", "row":
	default:
		return fmt.Errorf("%s: unknown section type %q", path, sec.Type)
	}

	if sec.Binding != nil {
		if err := validateBinding(sec.Binding, datasets, path); err != nil {
			return err
		}
	}

	switch sec.Type {
	case "kpi":
		for j, item := range sec.Items {
			if item.Binding != nil {
				if err := validateBinding(item.Binding, datasets, fmt.Sprintf("%s.items[%d]", path, j)); err != nil {
					return err
				}
			}
		}
	case "echarts":
		hasOption := len(sec.Option) > 0
		hasBinding := sec.Binding != nil
		if !hasOption && !hasBinding {
			return fmt.Errorf("%s: echarts section requires option or binding", path)
		}
	case "row":
		for j, child := range sec.Sections {
			if err := validateSection(child, datasets, fmt.Sprintf("%s.sections[%d]", path, j)); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateBinding(binding *Binding, datasets map[string]Dataset, path string) error {
	if binding.Dataset == "" {
		return fmt.Errorf("%s: binding.dataset is required", path)
	}
	if _, ok := datasets[binding.Dataset]; !ok {
		return fmt.Errorf("%s: binding references unknown dataset %q", path, binding.Dataset)
	}
	return nil
}
