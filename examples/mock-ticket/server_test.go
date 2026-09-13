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

func TestGetTicketByID(t *testing.T) {
	h := mockticket.NewHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"title": "VPN 挂了", "priority": "high"})
	res, err := http.Post(srv.URL+"/tickets", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", created)
	}
	if created["status"] != "open" {
		t.Fatalf("default status=%v, want open", created["status"])
	}

	res, err = http.Get(srv.URL + "/tickets/" + id)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if got["id"] != id || got["title"] != "VPN 挂了" {
		t.Fatalf("got=%v", got)
	}

	res, err = http.Get(srv.URL + "/tickets/missing")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id status=%d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestPatchTicketStatus(t *testing.T) {
	h := mockticket.NewHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"title": "打印机坏了"})
	res, err := http.Post(srv.URL+"/tickets", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	id, _ := created["id"].(string)

	patch, _ := json.Marshal(map[string]string{"status": "closed"})
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/tickets/"+id, bytes.NewReader(patch))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch status=%d", res.StatusCode)
	}
	var updated map[string]any
	if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if updated["status"] != "closed" {
		t.Fatalf("updated=%v", updated)
	}

	bad, _ := json.Marshal(map[string]string{"status": "not-a-status"})
	req, err = http.NewRequest(http.MethodPatch, srv.URL+"/tickets/"+id, bytes.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid status=%d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()
}
