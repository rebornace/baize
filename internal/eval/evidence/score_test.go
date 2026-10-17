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
		stem := strings.TrimSuffix(base, ".events.json")
		dot := strings.LastIndex(stem, ".")
		if dot < 0 {
			t.Fatalf("%s: expected <caseId>.<tag>.events.json", base)
		}
		id, tag := stem[:dot], stem[dot+1:]
		c, ok := cases[id]
		if !ok {
			t.Fatalf("%s: unknown case id %s", base, id)
		}
		statePath := strings.TrimSuffix(path, ".events.json") + ".state.json"
		if raw, err := os.ReadFile(statePath); err == nil {
			var snap evidence.StateSnapshot
			if err := json.Unmarshal(raw, &snap); err != nil {
				t.Fatalf("%s: state json: %v", base, err)
			}
			c.StateSnapshot = &snap
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
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

func TestScoreForbid(t *testing.T) {
	c := evidence.Case{Protocol: "V0", RequiredReads: []string{"list_tickets"}, Consequential: []string{"create_ticket"}, ForbidTools: []string{"create_ticket"}}
	tl := []evidence.ToolCall{{Name: "list_tickets", HasResult: true}, {Name: "create_ticket", HasResult: true}}
	res := evidence.Score(c, tl, "")
	if res.Pass || !contains(res.Codes, "FORBID") {
		t.Fatalf("%+v", res)
	}
}

func TestScoreArgContains(t *testing.T) {
	c := evidence.Case{Protocol: "V0", RequiredReads: []string{"get_ticket"}, Consequential: []string{"create_ticket"},
		ArgContains: []evidence.ArgContains{{Tool: "get_ticket", Substr: "demo-1"}}}
	tl := []evidence.ToolCall{{Name: "get_ticket", HasResult: true, ArgsText: `{"id":"demo-1"}`}}
	if !evidence.Score(c, tl, "").Pass {
		t.Fatal("want pass")
	}
	tl[0].ArgsText = `{"id":"other"}`
	res := evidence.Score(c, tl, "")
	if res.Pass || !contains(res.Codes, "ARG") {
		t.Fatalf("%+v", res)
	}
}

func TestScoreExpectState(t *testing.T) {
	c := evidence.Case{Protocol: "V0", RequiredReads: []string{"list_tickets"}, Consequential: []string{"create_ticket"},
		ExpectState: []evidence.ExpectTicket{{TitleContains: "链路", Status: "closed"}}}
	res := evidence.Score(c, nil, "")
	if !contains(res.Codes, "STATE") {
		t.Fatalf("%+v", res)
	}
	c.StateSnapshot = &evidence.StateSnapshot{Tickets: []evidence.StateTicket{{ID: "t1", Title: "链路评测", Status: "closed"}}}
	if !evidence.Score(c, []evidence.ToolCall{{Name: "list_tickets", HasResult: true}}, "").Pass {
		t.Fatal()
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
