package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPutConnectorGroupsHotApplies(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var applied []config.ConnectorGroup
	srv := NewServer(st, reg, nil)
	srv.Config = &config.Config{}
	srv.ApplyConnectorGroups = func(g []config.ConnectorGroup) { applied = g }

	body := `{"groups":[{"id":"finance","connectors":["expense","invoice"]}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/connector-groups", bytes.NewReader([]byte(body)))
	rr := httptest.NewRecorder()
	srv.handlePutConnectorGroups(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(applied) != 1 || applied[0].ID != "finance" {
		t.Fatalf("applied=%+v", applied)
	}
	raw, ok, err := st.GetSetting(store.SettingKeyConnectorGroups)
	if err != nil || !ok {
		t.Fatalf("persist ok=%v err=%v", ok, err)
	}
	var saved connectorGroupsBody
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Groups) != 1 {
		t.Fatalf("saved=%+v", saved)
	}
}
