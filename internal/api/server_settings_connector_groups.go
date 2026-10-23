package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

type connectorGroupView struct {
	ID         string   `json:"id"`
	Connectors []string `json:"connectors"`
}

type connectorGroupsBody struct {
	Groups []connectorGroupView `json:"groups"`
	// Source is "kv" when settings overlay is present, else "yaml".
	Source string `json:"source,omitempty"`
}

func normalizeConnectorGroups(in []connectorGroupView) ([]config.ConnectorGroup, error) {
	seenID := map[string]bool{}
	var out []config.ConnectorGroup
	for _, g := range in {
		id := strings.TrimSpace(g.ID)
		if id == "" {
			return nil, errBadRequest("group id required")
		}
		if seenID[id] {
			return nil, errBadRequest("duplicate group id: " + id)
		}
		seenID[id] = true
		seenConn := map[string]bool{}
		var conns []string
		for _, c := range g.Connectors {
			c = strings.TrimSpace(c)
			if c == "" || seenConn[c] {
				continue
			}
			seenConn[c] = true
			conns = append(conns, c)
		}
		if len(conns) == 0 {
			return nil, errBadRequest("group " + id + ": connectors required")
		}
		out = append(out, config.ConnectorGroup{ID: id, Connectors: conns})
	}
	return out, nil
}

type badRequestError string

func (e badRequestError) Error() string { return string(e) }

func errBadRequest(msg string) error { return badRequestError(msg) }

func groupsToView(groups []config.ConnectorGroup) []connectorGroupView {
	out := make([]connectorGroupView, 0, len(groups))
	for _, g := range groups {
		out = append(out, connectorGroupView{ID: g.ID, Connectors: append([]string(nil), g.Connectors...)})
	}
	return out
}

// effectiveConnectorGroups returns KV overlay when present, else YAML baseline.
func (s *Server) effectiveConnectorGroups() ([]config.ConnectorGroup, string, error) {
	raw, ok, err := s.Store.GetSetting(store.SettingKeyConnectorGroups)
	if err != nil {
		return nil, "", err
	}
	if ok && len(raw) > 0 && string(raw) != "null" && string(raw) != "[]" {
		var body connectorGroupsBody
		if err := json.Unmarshal(raw, &body); err != nil {
			// Tolerate bare array.
			var arr []connectorGroupView
			if err2 := json.Unmarshal(raw, &arr); err2 != nil {
				return nil, "", err
			}
			groups, nerr := normalizeConnectorGroups(arr)
			return groups, "kv", nerr
		}
		groups, nerr := normalizeConnectorGroups(body.Groups)
		if nerr != nil {
			return nil, "kv", nerr
		}
		return groups, "kv", nil
	}
	var yamlGroups []config.ConnectorGroup
	if s.Config != nil {
		yamlGroups = s.Config.ConnectorGroups
	}
	return yamlGroups, "yaml", nil
}

func (s *Server) applyConnectorGroupsLive(groups []config.ConnectorGroup) {
	if s.ApplyConnectorGroups != nil {
		s.ApplyConnectorGroups(groups)
	}
}

func (s *Server) handleGetConnectorGroups(w http.ResponseWriter, r *http.Request) {
	groups, source, err := s.effectiveConnectorGroups()
	if err != nil {
		if _, ok := err.(badRequestError); ok {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, connectorGroupsBody{
		Groups: groupsToView(groups),
		Source: source,
	})
}

func (s *Server) handlePutConnectorGroups(w http.ResponseWriter, r *http.Request) {
	var body connectorGroupsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON")
		return
	}
	groups, err := normalizeConnectorGroups(body.Groups)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	persist := connectorGroupsBody{Groups: groupsToView(groups)}
	raw, err := json.Marshal(persist)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(store.SettingKeyConnectorGroups, raw); err != nil {
		if writeIfSettingsKeyRequired(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.applyConnectorGroupsLive(groups)
	writeJSON(w, http.StatusOK, connectorGroupsBody{Groups: groupsToView(groups), Source: "kv"})
}
