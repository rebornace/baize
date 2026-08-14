package mockticket

import "os"

func ListenAddr() string {
	if v := os.Getenv("BAIZE_MOCK_TICKET_LISTEN"); v != "" {
		return v
	}
	return ":18080"
}
