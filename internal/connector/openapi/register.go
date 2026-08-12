package openapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// ErrToolConflict is returned when registering would overwrite another connector's tools.
var ErrToolConflict = errors.New("tool_conflict")

// RegisterConnector loads an OpenAPI connector into the store and tool registry.
func RegisterConnector(
	st store.Store,
	reg *tool.Registry,
	id, typ, specPath, baseURL string,
	requireApproval []string,
) (store.Connector, []tool.Info, error) {
	if typ == "" {
		typ = "openapi"
	}
	if typ != "openapi" {
		return store.Connector{}, nil, fmt.Errorf("unsupported connector type")
	}
	routes, err := LoadTools(specPath)
	if err != nil {
		return store.Connector{}, nil, fmt.Errorf("invalid_spec: %w", err)
	}
	names := make([]string, len(routes))
	for i, r := range routes {
		names[i] = r.Name
	}
	if reg.WouldConflict(id, names) {
		return store.Connector{}, nil, ErrToolConflict
	}
	reg.UnregisterConnector(id)
	approval := map[string]bool{}
	for _, n := range requireApproval {
		approval[n] = true
	}
	inv := &Invoker{BaseURL: baseURL, Tools: routes}
	for _, route := range routes {
		route := route
		name := route.Name
		reg.RegisterMeta(tool.Meta{
			Spec: llm.ToolSpec{
				Name:        route.Name,
				Description: route.Description,
				InputSchema: route.InputSchema,
			},
			ConnectorID: id,
			OperationID: route.OperationID,
			Method:      route.Method,
			Path:        route.Path,
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			res, err := inv.Invoke(ctx, name, args)
			if err != nil {
				return nil, true, err
			}
			return res.Content, res.IsError, nil
		}, approval[name])
	}
	c := store.Connector{
		ID:              id,
		Type:            typ,
		Spec:            specPath,
		BaseURL:         baseURL,
		RequireApproval: requireApproval,
	}
	st.UpsertConnector(c)
	return c, filterInfos(reg, id), nil
}

func filterInfos(reg *tool.Registry, connectorID string) []tool.Info {
	all := reg.List()
	out := make([]tool.Info, 0, len(all))
	for _, info := range all {
		if info.ConnectorID == connectorID {
			out = append(out, info)
		}
	}
	return out
}
