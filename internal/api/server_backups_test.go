package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackupAPIUnsupportedAndValidation(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	list := httptest.NewRequest(http.MethodGet, "/v0/settings/backups", nil)
	list.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, list)
	if rr.Code != http.StatusOK {
		t.Fatalf("list=%d", rr.Code)
	}

	create := httptest.NewRequest(http.MethodPost, "/v0/settings/backups", nil)
	create.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, create)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("memory backup=%d", rr.Code)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "bad.db")
	_, _ = part.Write([]byte("not a database"))
	_ = mw.Close()
	restore := httptest.NewRequest(http.MethodPost, "/v0/settings/backups/restore", &body)
	restore.Header.Set("Content-Type", mw.FormDataContentType())
	restore.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, restore)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad restore=%d %s", rr.Code, rr.Body.String())
	}
}
