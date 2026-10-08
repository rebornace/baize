package run

import (
	"context"
	"strconv"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// narrowKnobs builds knobs with tool routing on (which now narrows directly;
// there is no observe-only mode).
func narrowKnobs(threshold, preTopK int) fakeKnobs {
	return fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolRoutingEnabled: true,
		DecideToolThreshold:      threshold,
		DecideToolPreTopK:        preTopK,
	}}
}

func newNarrowEngine(t *testing.T, nTools int, decider decide.Ask, settings fakeKnobs, input, source string) (*Engine, *store.Run, *int) {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	noop := func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}
	for i := 0; i < nTools; i++ {
		name := "tool_" + strconv.Itoa(i)
		reg.RegisterSpecApproved(llm.ToolSpec{
			Name: name, Description: "does thing " + strconv.Itoa(i), Source: source,
		}, noop, false)
	}
	sent := 0
	llmStub := &captureLLM{onChat: func(_ []llm.Message, tools []llm.ToolSpec) llm.Message {
		sent = len(tools)
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg, Decider: decider, Settings: settings,
	}
	return eng, r, &sent
}

// AC-1: when routing is on above the threshold, the model must receive only
// the narrowed set, and the system-routing decision must have been consulted.
func TestToolNarrowNarrowsAndConsultsSystemRouting(t *testing.T) {
	// Every fixture tool belongs to sys_a; system routing returns it.
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Verdict: decide.VerdictYes, Values: []string{"sys_a"}, Source: decide.SourceRemote},
	}}
	eng, r, sent := newNarrowEngine(t, 40, decider, narrowKnobs(12, 5), "thing 3", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "thing 3"},
	}); err != nil {
		t.Fatal(err)
	}

	if *sent != 5 {
		t.Fatalf("must send only the narrowed set, sent=%d want 5", *sent)
	}
	sawSystem := false
	for _, c := range decider.calls {
		if c.Kind == decide.KindSystemTargets {
			sawSystem = true
			if len(c.Options) != 1 || c.Options[0] != "sys_a" {
				t.Fatalf("system options=%v want [sys_a]", c.Options)
			}
		}
	}
	if !sawSystem {
		t.Fatal("system routing must be consulted")
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	var narrow *store.Event
	for i := range evs {
		if evs[i].Type == EventDecideToolNarrow {
			narrow = &evs[i]
		}
	}
	if narrow == nil {
		t.Fatalf("missing decide.tool_narrow event; evs=%+v", evs)
	}
	if asInt(narrow.Data, "total") != 40 {
		t.Fatalf("total=%v want 40", narrow.Data["total"])
	}
	if asInt(narrow.Data, "sent_count") != 5 {
		t.Fatalf("sent_count=%v want 5", narrow.Data["sent_count"])
	}
	if asInt(narrow.Data, "prefilter_count") != 5 {
		t.Fatalf("prefilter_count=%v want 5", narrow.Data["prefilter_count"])
	}
	if b, _ := narrow.Data["systems_degraded"].(bool); b {
		t.Fatalf("systems_degraded=%v want false", narrow.Data["systems_degraded"])
	}
}

// AC-2: at or below the threshold the layer is not consulted and no narrow
// event is written (full set sent).
func TestToolNarrowSkipsBelowThreshold(t *testing.T) {
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Values: []string{"sys_a"}},
	}}
	eng, r, sent := newNarrowEngine(t, 10, decider, narrowKnobs(12, 5), "thing", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "thing"},
	}); err != nil {
		t.Fatal(err)
	}
	if *sent != 10 {
		t.Fatalf("sent=%d want 10", *sent)
	}
	if len(decider.calls) != 0 {
		t.Fatalf("decider must not be consulted below threshold, calls=%+v", decider.calls)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventDecideToolNarrow {
			t.Fatalf("no narrow event expected below threshold; ev=%+v", ev)
		}
	}
}

// AC-3: if keyword matching finds nothing in common with the catalog, it fails
// open and sends the full set so the model is never starved.
func TestToolNarrowFailsOpenOnNoMatch(t *testing.T) {
	// System routing degrades (abstains); combined with no keyword overlap the
	// run must fall back to the full catalog.
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Verdict: decide.VerdictYes, Degraded: true, Source: decide.SourceFallback},
	}}
	eng, r, sent := newNarrowEngine(t, 40, decider, narrowKnobs(12, 5), "zzz qqq unrelated gibberish", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "zzz qqq unrelated gibberish"},
	}); err != nil {
		t.Fatal(err)
	}

	if *sent != 40 {
		t.Fatalf("must fail open to full set on no match, sent=%d want 40", *sent)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	var narrow *store.Event
	for i := range evs {
		if evs[i].Type == EventDecideToolNarrow {
			narrow = &evs[i]
		}
	}
	if narrow == nil {
		t.Fatal("missing narrow event")
	}
	if b, _ := narrow.Data["prefilter_empty"].(bool); !b {
		t.Fatalf("prefilter_empty=%v want true", narrow.Data["prefilter_empty"])
	}
	if asInt(narrow.Data, "sent_count") != 40 {
		t.Fatalf("sent_count=%v want 40", narrow.Data["sent_count"])
	}
}

// AC-4: with the master switch off the layer never runs.
func TestToolNarrowDisabledByDefault(t *testing.T) {
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Values: []string{"sys_a"}},
	}}
	off := fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled: false, DecideToolRoutingEnabled: true,
		DecideToolThreshold: 12,
	}}
	eng, r, _ := newNarrowEngine(t, 15, decider, off, "thing", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "thing"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(decider.calls) != 0 {
		t.Fatalf("decider must not run when master switch off, calls=%+v", decider.calls)
	}
}

func TestHasDeleteIntentShortChinese(t *testing.T) {
	// User phrasing often omits 除:「删用户」≠ substring「删除」.
	if !hasDeleteIntent("后台删用户id为116的账号") {
		t.Fatal("删用户 must count as delete intent")
	}
	if !hasDeleteIntent("删除用户") {
		t.Fatal("删除用户 must count as delete intent")
	}
	if !hasDeleteIntent("please delete user 116") {
		t.Fatal("English delete must count as delete intent")
	}
}

func TestExpandAllowedForHTTPIntentAddsDeleteBackends(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "login", Method: "POST", Path: "/login", Source: "app-api"},
		{Name: "register", Method: "POST", Path: "/register", Source: "app-api"},
		{Name: "delete_record", Method: "DELETE", Path: "/items/{id}", Source: "admin-api"},
		{Name: "UsersAdminController_remove", Method: "POST", Path: "/users/{id}", Source: "pets-admin"},
	}
	got := expandAllowedForHTTPIntent("删除这条记录", specs, map[string]bool{"app-api": true})
	if !got["admin-api"] || !got["pets-admin"] || !got["app-api"] {
		t.Fatalf("delete intent must reopen DELETE and POST-remove backends, got %v", got)
	}
}

func TestPreserveHTTPIntentToolsUsesMethodNotSuffix(t *testing.T) {
	full := []llm.ToolSpec{
		{Name: "list_people", Method: "GET", Path: "/people", Description: "分页", Source: "admin-api"},
		{Name: "destroy_person", Method: "DELETE", Path: "/people/{id}", Description: "destroy", Source: "admin-api"},
		{Name: "UsersAdminController_remove", Method: "POST", Path: "/admin/users/{id}", Description: "", Source: "pets-admin"},
	}
	picked := []llm.ToolSpec{full[0]}
	out := preserveHTTPIntentTools("delete this person", full, map[string]bool{
		"admin-api": true, "pets-admin": true,
	}, picked)
	names := map[string]bool{}
	for _, s := range out {
		names[s.Name] = true
	}
	if !names["destroy_person"] {
		t.Fatalf("DELETE op must be preserved, got %v", names)
	}
	if !names["UsersAdminController_remove"] {
		t.Fatalf("POST …_remove must be preserved as destructive, got %v", names)
	}
}

// Auth-only system pick must not hide admin remove when the user asks to delete.
func TestNarrowToolsDeleteReopensAdminPOSTRemove(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	noop := func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "AuthController_login", Description: "用户登录"},
		ConnectorID: "app-api", Method: "POST", Path: "/auth/login",
	}, noop, false)
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "AuthController_register", Description: "用户注册"},
		ConnectorID: "app-api", Method: "POST", Path: "/auth/register",
	}, noop, false)
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "AdminAuthController_me", Description: "当前用户"},
		ConnectorID: "app-api", Method: "GET", Path: "/admin/me",
	}, noop, false)
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "UsersAdminController_remove", Description: ""},
		ConnectorID: "pets-admin", Method: "POST", Path: "/admin/users/{id}",
	}, noop, false)
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "UsersAdminController_findPage", Description: "用户分页"},
		ConnectorID: "pets-admin", Method: "GET", Path: "/admin/users",
	}, noop, false)

	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {
			Verdict: decide.VerdictYes, Values: []string{"app-api"}, Source: decide.SourceRemote,
		},
	}}
	llmStub := &captureLLM{onChat: func(_ []llm.Message, _ []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "ok"}
	}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "后台删用户id为116的账号"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg, Decider: decider,
		Settings: narrowKnobs(2, 8),
	}
	sent := eng.narrowTools(context.Background(), r.ID, 0, eng.specsForRun(r.ID), nil)
	names := map[string]bool{}
	for _, s := range sent {
		names[s.Name] = true
	}
	if !names["UsersAdminController_remove"] {
		t.Fatalf("narrowed set must include UsersAdminController_remove for「删用户」, got %v", names)
	}
}

func TestDeterministicSystemsHighDFAuthTokenDoesNotHideAdmin(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "login", Description: "用户登录", Source: "app-api"},
		{Name: "register", Description: "用户注册", Source: "app-api"},
		{Name: "profile", Description: "用户资料", Source: "app-api"},
		{Name: "reset", Description: "用户找回密码", Source: "app-api"},
		{Name: "destroy_person", Method: "DELETE", Path: "/people/{id}", Source: "admin-api"},
	}
	systems := []string{"app-api", "admin-api"}
	q := queryContentTokens("删除用户")
	got := map[string]bool{}
	for _, s := range deterministicSystems(specs, systems, q) {
		got[s] = true
	}
	if got["app-api"] && !got["admin-api"] {
		t.Fatalf("high-DF「用户」on auth must not be the only forced system, got %v", got)
	}
}

// AC-5: protected system tools (activate_skill) survive narrowing even though
// keyword matching would otherwise drop them. Plain Source="" tools are not
// blanket-preserved, and a filtered connector tool stays dropped.
func TestPreserveEssentialTools(t *testing.T) {
	full := []llm.ToolSpec{
		{Name: "builtin_a"},            // Source="" -> not forced
		{Name: skill.ActivateToolName}, // protected -> appended
		{Name: "b", Source: "x"},       // connector tool -> not appended
	}
	picked := []llm.ToolSpec{{Name: "z", Source: "z"}}
	out := preserveEssentialTools(picked, full)
	names := make(map[string]bool, len(out))
	for _, s := range out {
		names[s.Name] = true
	}
	if !names[skill.ActivateToolName] {
		t.Fatalf("activate_skill must be preserved, got %v", names)
	}
	for _, dropped := range []string{"builtin_a", "b"} {
		if names[dropped] {
			t.Fatalf("%q must not be blanket re-added, got %v", dropped, names)
		}
	}
}

// Skill `tools:` is a floor through DP-2a: a default skill's declared tools
// stay callable even when the turn's keywords route to another connector.
func TestNarrowToolsKeepsSkillDeclaredTools(t *testing.T) {
	cat := loadTestCatalog(t, map[string]struct {
		desc  string
		tools []string
		body  string
	}{
		"demo": {desc: "analytics", tools: []string{"create_analysis_page"}, body: "make a page"},
	})
	st := store.NewMemory()
	reg := tool.NewRegistry()
	noop := func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}
	reg.RegisterSpecApproved(llm.ToolSpec{
		Name: "create_analysis_page", Description: "build an analysis page",
	}, noop, false)
	for i := 0; i < 20; i++ {
		reg.RegisterSpecApproved(llm.ToolSpec{
			Name: "pet_find_" + strconv.Itoa(i), Description: "宠物查询 " + strconv.Itoa(i), Source: "pet-api",
		}, noop, false)
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "查一下宠物体重"})
	if err != nil {
		t.Fatal(err)
	}
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Verdict: decide.VerdictYes, Values: []string{"pet-api"}, Source: decide.SourceRemote},
	}}
	eng := &Engine{
		Store: st, Tools: reg, Skills: cat, Decider: decider, Settings: narrowKnobs(12, 4),
	}
	eng.beginRunSkills(r.ID, []string{"demo"}, "sys")
	specs := eng.specsForRun(r.ID)
	out := eng.narrowTools(context.Background(), r.ID, 0, specs, []llm.Message{
		{Role: llm.RoleUser, Content: "查一下宠物体重"},
	})
	got := specSet(out)
	if !got["create_analysis_page"] {
		t.Fatalf("skill-declared tool must survive routing; got %v", got)
	}
}
