package capfill

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/store"
)

const (
	ProposeName  = "propose_plugin"
	MaxGoalRunes = 500
	MaxGapRunes  = 4000
	MaxRiskRunes = 2000
)

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

type PluginToolDraft struct {
	Name        string
	Description string
}

type PluginDraft struct {
	ConnectorID          string
	Goal                 string
	Gap                  string
	Tools                []PluginToolDraft
	ReadOnly             bool
	Risk                 string
	BaseURL              string
	ExecutionCallbackURL string
}

func ValidateProposePluginArgs(args map[string]any, _ time.Time) error {
	connectorID, err := requiredStringArg(args, "connector_id")
	if err != nil {
		return err
	}
	if err := validatePluginID(connectorID); err != nil {
		return err
	}
	goal, err := requiredStringArg(args, "goal")
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(goal) > MaxGoalRunes {
		return fmt.Errorf("goal exceeds %d runes", MaxGoalRunes)
	}
	gap, err := requiredStringArg(args, "gap")
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(gap) > MaxGapRunes {
		return fmt.Errorf("gap exceeds %d runes", MaxGapRunes)
	}
	risk, err := requiredStringArg(args, "risk")
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(risk) > MaxRiskRunes {
		return fmt.Errorf("risk exceeds %d runes", MaxRiskRunes)
	}
	tools, err := parsePluginToolsArg(args["tools"])
	if err != nil {
		return err
	}
	if len(tools) == 0 {
		return fmt.Errorf("tools required")
	}
	for _, t := range tools {
		if err := validatePluginID(t.Name); err != nil {
			return fmt.Errorf("tool name: %w", err)
		}
	}
	readOnly, err := requiredBoolArg(args, "read_only")
	if err != nil {
		return err
	}
	if !readOnly {
		return fmt.Errorf("read_only must be true")
	}
	if err := optionalStringField(args, "base_url"); err != nil {
		return err
	}
	if err := optionalStringField(args, "execution_callback_url"); err != nil {
		return err
	}
	return nil
}

func DraftFromProposePluginArgs(args map[string]any) (PluginDraft, error) {
	connectorID, err := requiredStringArg(args, "connector_id")
	if err != nil {
		return PluginDraft{}, err
	}
	goal, err := requiredStringArg(args, "goal")
	if err != nil {
		return PluginDraft{}, err
	}
	gap, err := requiredStringArg(args, "gap")
	if err != nil {
		return PluginDraft{}, err
	}
	risk, err := requiredStringArg(args, "risk")
	if err != nil {
		return PluginDraft{}, err
	}
	tools, err := parsePluginToolsArg(args["tools"])
	if err != nil {
		return PluginDraft{}, err
	}
	readOnly, err := requiredBoolArg(args, "read_only")
	if err != nil {
		return PluginDraft{}, err
	}
	baseURL, err := optionalStringArg(args, "base_url")
	if err != nil {
		return PluginDraft{}, err
	}
	callbackURL, err := optionalStringArg(args, "execution_callback_url")
	if err != nil {
		return PluginDraft{}, err
	}
	return PluginDraft{
		ConnectorID:          connectorID,
		Goal:                 goal,
		Gap:                  gap,
		Tools:                tools,
		ReadOnly:             readOnly,
		Risk:                 risk,
		BaseURL:              baseURL,
		ExecutionCallbackURL: callbackURL,
	}, nil
}

func CheckPlaceholderConflicts(st store.Store, d PluginDraft) error {
	if st == nil {
		return fmt.Errorf("store unavailable")
	}
	if _, err := st.GetConnector(d.ConnectorID); err == nil {
		return fmt.Errorf("connector %q already exists", d.ConnectorID)
	}
	for _, t := range d.Tools {
		if _, err := st.GetTool(t.Name); err == nil {
			return fmt.Errorf("tool %q already exists", t.Name)
		}
	}
	return nil
}

func InstallPluginPlaceholder(st store.Store, d PluginDraft) error {
	if err := CheckPlaceholderConflicts(st, d); err != nil {
		return err
	}
	if !d.ReadOnly {
		return fmt.Errorf("plugin draft must declare read_only=true")
	}
	st.UpsertConnector(store.Connector{
		ID:                   d.ConnectorID,
		Type:                 "http",
		BaseURL:              d.BaseURL,
		ExecutionCallbackURL: d.ExecutionCallbackURL,
	})
	ro := true
	for _, t := range d.Tools {
		st.UpsertTool(store.Tool{
			ConnectorID:  d.ConnectorID,
			Name:         t.Name,
			Source:       store.ToolSourcePlugin,
			Enabled:      false,
			Description:  t.Description,
			ReadOnlyHint: &ro,
		})
	}
	return nil
}

func validatePluginID(id string) error {
	if !pluginIDPattern.MatchString(id) {
		return fmt.Errorf("invalid id %q", id)
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

func optionalStringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(s), nil
}

func optionalStringField(args map[string]any, key string) error {
	_, err := optionalStringArg(args, key)
	return err
}

func requiredBoolArg(args map[string]any, key string) (bool, error) {
	v, ok := args[key]
	if !ok {
		return false, fmt.Errorf("%s required", key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return b, nil
}

func parsePluginToolsArg(raw any) ([]PluginToolDraft, error) {
	if raw == nil {
		return nil, fmt.Errorf("tools required")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("tools must be an array")
	}
	out := make([]PluginToolDraft, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tools must be an array of objects")
		}
		name, err := requiredStringArg(m, "name")
		if err != nil {
			return nil, err
		}
		desc := ""
		if dRaw, ok := m["description"]; ok && dRaw != nil {
			d, ok := dRaw.(string)
			if !ok {
				return nil, fmt.Errorf("description must be a string")
			}
			desc = strings.TrimSpace(d)
		}
		out = append(out, PluginToolDraft{Name: name, Description: desc})
	}
	return out, nil
}
