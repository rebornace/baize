package mockproject

import "os"

func ListenAddr() string {
	if v := os.Getenv("BAIZE_MOCK_PROJECT_LISTEN"); v != "" {
		return v
	}
	return ":18081"
}
