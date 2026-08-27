package main

import (
	"log"
	"net/http"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
)

func main() {
	addr := mockticket.ListenAddr()
	log.Printf("mock-ticket listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mockticket.NewHandler()))
}
