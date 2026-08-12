package mockticket_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
)

func TestCreateAndListTickets(t *testing.T) {
	h := mockticket.NewHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"title": "VPN 挂了", "priority": "high"})
	res, err := http.Post(srv.URL+"/tickets", "application/json", bytes.NewReader(body))
	if err != nil || res.StatusCode != 201 {
		t.Fatalf("create: %v %v", err, res)
	}
	res, err = http.Get(srv.URL + "/tickets")
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	json.NewDecoder(res.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("list=%v", list)
	}
}

func TestHealthz(t *testing.T) {
	h := mockticket.NewHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body=%v", body)
	}
}
