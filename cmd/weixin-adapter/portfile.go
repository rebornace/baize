package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeListenPortFile writes a single-line host:port for baize port discovery.
func writeListenPortFile(path, addr string) error {
	path = strings.TrimSpace(path)
	addr = strings.TrimSpace(addr)
	if path == "" {
		return fmt.Errorf("empty port-file path")
	}
	if addr == "" {
		return fmt.Errorf("empty listen addr")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(addr+"\n"), 0o644)
}
