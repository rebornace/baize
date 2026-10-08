package toolretrieval

import "testing"

func TestClassifyPullErrorVersionTooOld(t *testing.T) {
	msg := "pull model manifest: 412: The model you are attempting to pull requires a newer version of Ollama."
	if got := classifyPullError(msg); got != "ollama_version_too_old" {
		t.Fatalf("got %q", got)
	}
	if got := classifyPullError("connection refused"); got != "connection refused" {
		t.Fatalf("passthrough got %q", got)
	}
}
