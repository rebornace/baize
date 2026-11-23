package main

import (
	"log"
	"net/http"
	"os"

	httppluginex "github.com/rebornace/baize/examples/http-plugin"
)

func main() {
	addr := ":19090"
	if v := os.Getenv("BAIZE_HTTP_PLUGIN_LISTEN"); v != "" {
		addr = v
	}
	log.Fatal(http.ListenAndServe(addr, httppluginex.NewHandler()))
}
