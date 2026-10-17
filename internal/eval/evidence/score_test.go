package evidence_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/eval/evidence"
)

func evidenceEvalDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "scripts", "evidence-eval"))
}

func loadCases(t *testing.T, dir string) map[string]evidence.Case {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var list []evidence.Case
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]evidence.Case, len(list))
	for _, c := range list {
		byID[c.ID] = c
	}
	return byID
}

func loadEvents(t *testing.T, path string) []evidence.Event {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var evs []evidence.Event
	if err := json.Unmarshal(raw, &evs); err != nil {
		t.Fatal(err)
	}
	return evs
}

func contains(codes []string, want string) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}

func TestScoreFixturesAlignPS1(t *testing.T) {
	dir := evidenceEvalDir(t)
	cases := loadCases(t, dir)
	fixtures, err := filepath.Glob(filepath.Join(dir, "fixtures", "*.events.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures")
	}
	for _, path := range fixtures {
		base := filepath.Base(path)
		parts := strings.Split(strings.TrimSuffix(base, ".events.json"), ".")
		id, tag := parts[0], parts[len(parts)-1]
		c, ok := cases[id]
		if !ok {
			t.Fatalf("%s: unknown case id %s", base, id)
		}
		evs := loadEvents(t, path)
		tl := evidence.ToolTimeline(evs)
		res := evidence.Score(c, tl, "")
		wantPass := tag == "pass"
		if res.Pass != wantPass {
			t.Fatalf("%s: pass want %v got %v codes=%v", base, wantPass, res.Pass, res.Codes)
		}
		if !wantPass {
			want := strings.ToUpper(tag)
			if !contains(res.Codes, want) {
				t.Fatalf("%s: want code %s in %v", base, want, res.Codes)
			}
		}
	}
}

func TestScoreRunStatusTimeout(t *testing.T) {
	dir := evidenceEvalDir(t)
	cases := loadCases(t, dir)
	c := cases["EV-V0-list-only"]
	path := filepath.Join(dir, "fixtures", "EV-V0-list-only.pass.events.json")
	evs := loadEvents(t, path)
	tl := evidence.ToolTimeline(evs)
	res := evidence.Score(c, tl, "timeout")
	if res.Pass {
		t.Fatal("expected fail with TIMEOUT")
	}
	if !contains(res.Codes, "TIMEOUT") {
		t.Fatalf("want TIMEOUT in %v", res.Codes)
	}
}
