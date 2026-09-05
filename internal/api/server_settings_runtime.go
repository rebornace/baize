package api

import "net/http"

// Runtime settings / credentials endpoints. Task 4 registers the routes and
// ACL rules; these handlers are 501 stubs until task 5/6 wires the real
// read/patch implementations backed by runtimecfg.Holder.

func (s *Server) handleGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5/6")
}

func (s *Server) handlePatchRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5/6")
}

func (s *Server) handleGetCredentials(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5/6")
}

func (s *Server) handlePatchCredentials(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5/6")
}
