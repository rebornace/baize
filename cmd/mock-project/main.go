// Command mock-project runs the EV-PRES cross-domain project API.
//
//	go run ./cmd/mock-project
package main

import (
	"log"
	"net/http"

	mockproject "github.com/rebornace/baize/examples/mock-project"
)

func main() {
	addr := mockproject.ListenAddr()
	log.Printf("mock-project on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mockproject.NewHandler()))
}
