package run

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

// toolNames returns the candidate tool names in spec order.
func toolNames(specs []llm.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

// narrowTools consults the decision layer's system routing and returns the
// specs actually sent to the main model. First a tiny system-routing decision
// chooses which connectors the turn needs; then the deterministic keyword
// prefilter runs WITHIN each chosen system (local IDF) and merges slots
// round-robin. Built-in tools (Source="") and activate_skill are always
// retained. If system routing abstains or keyword matching finds nothing, it
// fails open (all systems / full set) so ordinary chit-chat is never starved.
//
// OnFail is VerdictYes: if system routing abstains, every connector remains
// eligible.
func (e *Engine) narrowTools(ctx context.Context, runID string, turn int, specs []llm.ToolSpec, messages []llm.Message) []llm.ToolSpec {
	// The decision model must see what the user actually wants; the request is
	// capped (cheap probe).
	userRequest := ""
	if runRec, err := e.Store.GetRun(runID); err == nil && runRec != nil {
		userRequest = strings.TrimSpace(runRec.Input)
		if r := []rune(userRequest); len(r) > decideProbeMaxRunes {
			userRequest = string(r[:decideProbeMaxRunes])
		}
	}

	// Trajectory-aware narrowing: after the first ReAct step the decision
	// model must also see the compact trail of what has already been tried and
	// what it returned, so the candidate set can follow the plan as it unfolds
	// (e.g. product already listed, now fetch its SKU stock). Without this a
	// turn-0 pick can never cover tools the model discovers mid-execution.
	trajectory := buildCompactTrajectory(messages, decideTrajectoryMaxRunes)
	matchQuery := userRequest
	if trajectory != "" {
		matchQuery = userRequest + "\n" + trajectory
	}

	// --- System routing --------------------------------------------------
	// Choose among connector ids (a handful), far easier than choosing a tool
	// from hundreds. Built-in tools (Source="") are not a routable system;
	// they are always kept.
	systemIDs := connectorSources(specs)
	sysContext := "用户这一轮请求：\n" + userRequest + "\n"
	if trajectory != "" {
		sysContext += "\n到此为止的执行轨迹（精简）：\n" + trajectory + "\n"
	}
	sysAns, sysErr := e.Decider.Ask(ctx, decide.Question{
		Kind:         decide.KindSystemTargets,
		Context:      sysContext,
		Options:      systemIDs,
		Descriptions: buildSystemDescriptions(specs, systemIDs),
		OnFail:       decide.VerdictYes,
		TraceID:      runID,
	})
	if degradedFromAsk(sysAns, sysErr) {
		e.emitDecideDegraded(runID, decide.KindSystemTargets, map[string]any{
			"source": sysAns.Source,
			"turn":   turn,
		})
	}
	pickedSystems := validSystemNames(sysAns.Values, systemIDs)
	// Hybrid routing: systems hit deterministically by the query's own
	// discriminative domain words (宠物/订单…) are FORCED and can never be
	// removed by the model; the model's pick is unioned on top as a supplement.
	queryToks := queryContentTokens(matchQuery)
	forcedSystems := deterministicSystems(specs, systemIDs, queryToks)
	// Fail open: the model abstained/degraded AND nothing was forced => every
	// connector remains eligible (flat-catalog behavior). Otherwise restrict
	// to the forced ∪ model systems.
	var allowed map[string]bool
	if sysAns.Degraded && len(forcedSystems) == 0 {
		allowed = nil
	} else {
		allowed = make(map[string]bool)
		for _, s := range forcedSystems {
			allowed[s] = true
		}
		if !sysAns.Degraded {
			for _, s := range pickedSystems {
				allowed[s] = true
			}
		}
		if len(allowed) == 0 {
			allowed = nil
		}
	}

	// --- Level 2: per-system keyword prefilter with slot quota -----------
	preSpecs, noMatch := prefilterToolsBySystem(matchQuery, specs, allowed, e.effectiveDecidePreTopK())
	// Category-listing floor: a generic entity word ("用户"/"宠物") is too
	// common within its own system for the backend *_findPage tool to survive
	// IDF ranking, yet an explicit list/page request needs exactly it. Force
	// the matching admin pagination tool into the set so basic list queries
	// can never be silently starved of the right tool.
	preSpecs = preserveCategoryAdminTools(matchQuery, specs, allowed, preSpecs)
	// Auth/session floor: when a system is selected, its login + session-probe
	// primitives are always kept so the model can recover from an expired
	// session (401) via *_me / *_login without the query having to mention
	// login. There are very few such tools, so the cost is negligible.
	preSpecs = preserveAuthTools(specs, allowed, preSpecs)
	preNames := toolNames(preSpecs)

	sent := specs
	sentCount := len(specs)
	// Fail open: keyword matching found nothing in common, keep full set
	// rather than send an empty tool list. Otherwise retain protected system
	// tools (activate_skill) alongside the narrowed set.
	if !noMatch && len(preSpecs) > 0 {
		sent = preserveEssentialTools(preSpecs, specs)
		sentCount = len(sent)
	}
	data := map[string]any{
		"turn":             turn,
		"total":            len(specs),
		"systems":          systemIDs,
		"systems_picked":   pickedSystems,
		"systems_source":   sysAns.Source,
		"systems_degraded": sysAns.Degraded,
		"prefilter":        preNames,
		"prefilter_count":  len(preNames),
		"prefilter_empty":  noMatch,
		"tool_choice":      e.effectiveDecideToolChoice(),
		"sent_count":       sentCount,
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventDecideToolNarrow,
		Data: data,
	})
	return sent
}

// systemDescTerms is how many discriminative domain words each system
// description carries to the routing model.
const systemDescTerms = 8

// buildSystemDescriptions auto-derives a short domain summary for each
// connector from its tools' names/descriptions, since connectors carry no
// human description and a bare id ("mall") lets the model mis-route. It scores
// tokens by within-system frequency times cross-system IDF: words frequent in
// one system but absent from the others (宠物/订单) describe it, while words
// shared across systems (查询/Controller) are downweighted. Stopwords removed.
func buildSystemDescriptions(specs []llm.ToolSpec, systems []string) map[string]string {
	ranked := rankSystemTerms(specs, systems)
	out := make(map[string]string, len(systems))
	for _, sys := range systems {
		k := systemDescTerms
		if len(ranked[sys]) < k {
			k = len(ranked[sys])
		}
		terms := make([]string, 0, k)
		for i := 0; i < k; i++ {
			terms = append(terms, ranked[sys][i].t)
		}
		out[sys] = strings.Join(terms, "、")
	}
	return out
}

type systemTerm struct {
	t string
	s float64
}

// rankSystemTerms scores every content token per connector by within-system
// frequency times cross-system IDF: words frequent in one system but absent
// from the others (宠物/订单) describe it; words shared across systems
// (查询/Controller) are downweighted. Returns terms per system, score desc.
func rankSystemTerms(specs []llm.ToolSpec, systems []string) map[string][]systemTerm {
	members := make(map[string][]llm.ToolSpec)
	for _, s := range specs {
		if s.Source != "" {
			members[s.Source] = append(members[s.Source], s)
		}
	}
	systemsContaining := make(map[string]int)
	withinCount := make(map[string]map[string]int)
	for _, sys := range systems {
		counts := make(map[string]int)
		for _, s := range members[sys] {
			for t := range tokenSet(s.Name + " " + s.Description) {
				if queryStop[t] {
					continue
				}
				counts[t]++
			}
		}
		withinCount[sys] = counts
		for t := range counts {
			systemsContaining[t]++
		}
	}

	n := float64(len(systems))
	out := make(map[string][]systemTerm, len(systems))
	for _, sys := range systems {
		var ranked []systemTerm
		for t, f := range withinCount[sys] {
			idf := 1.0 + math.Log(n/float64(systemsContaining[t]))
			ranked = append(ranked, systemTerm{t, float64(f) * idf})
		}
		sort.SliceStable(ranked, func(a, b int) bool {
			if ranked[a].s != ranked[b].s {
				return ranked[a].s > ranked[b].s
			}
			return ranked[a].t < ranked[b].t
		})
		out[sys] = ranked
	}
	return out
}

// deterministicSystems returns every system owning a content (non-stopword)
// token that also appears in the query. This is the forced floor the model can
// only add to, never remove:
//   - a token exclusive to one system forces that one system (宠物 -> its
//     backend), which removes the model's "mall" bias;
//   - a genuinely shared entity token (用户 exists in two backends) forces ALL
//     owners, since the query gives no basis to choose and dropping the correct
//     one starves it; the per-system ranking then decides which tool to show.
//
// Generic/filler words are already removed as stopwords so they cannot force
// every system.
func deterministicSystems(specs []llm.ToolSpec, systems, queryTokens []string) []string {
	systemsByToken := make(map[string]map[string]bool)
	for _, s := range specs {
		if s.Source == "" {
			continue
		}
		for t := range tokenSet(s.Name + " " + s.Description) {
			if queryStop[t] {
				continue
			}
			if systemsByToken[t] == nil {
				systemsByToken[t] = make(map[string]bool)
			}
			systemsByToken[t][s.Source] = true
		}
	}
	forced := make(map[string]bool)
	totalSystems := len(systems)
	for _, qt := range queryTokens {
		owners := systemsByToken[qt]
		// A token present in EVERY system carries no routing signal (id,
		// generic operation words that survived the stoplist) and forcing all
		// owners would just fail-open every run. Only tokens belonging to a
		// STRICT SUBSET of systems force those owners: exclusive tokens pin
		// one system (dashboard) and shared-but-not-universal entity tokens
		// (用户 in two of three) preserve the genuine ambiguity.
		if len(owners) == 0 || len(owners) >= totalSystems {
			continue
		}
		for sys := range owners {
			forced[sys] = true
		}
	}
	var out []string
	for _, sys := range systems {
		if forced[sys] {
			out = append(out, sys)
		}
	}
	return out
}

// queryContentTokens returns the query's non-stopword content tokens, using the
// same tokenization as the prefilter.
func queryContentTokens(query string) []string {
	var out []string
	for t := range cleanQueryTokens(tokenSet(query)) {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// listIntentPatterns detect an explicit "show me a paged list" request.
var listIntentPatterns = []string{
	"列出", "列表", "分页", "前几", "多少个", "几条",
	"page", "list",
}

// detailIntentPatterns detect an explicit single-record detail request.
var detailIntentPatterns = []string{
	"详情", "详细", "具体信息",
	"detail", "getone",
}

// preserveCategoryAdminTools forces the backend admin read tool matching a
// query's entity word into the prefilter:
//   - list/page intent  -> *_findPage
//   - detail/id intent  -> *_findOne
//
// Generic entity words (用户/宠物) are so common within their own system that
// these admin tools' IDF score cannot make the cut, but an explicit listing or
// detail request needs exactly that tool. The tool must share a content token
// with the query and belong to an allowed system, so unrelated admin reads are
// never pulled in.
func preserveCategoryAdminTools(query string, full []llm.ToolSpec, allowed map[string]bool, picked []llm.ToolSpec) []llm.ToolSpec {
	suffix := ""
	switch {
	case hasListIntent(query):
		suffix = "_findPage"
	case hasDetailIntent(query):
		suffix = "_findOne"
	default:
		return picked
	}
	qToks := cleanQueryTokens(tokenSet(query))
	have := make(map[string]bool, len(picked))
	for _, s := range picked {
		have[s.Name] = true
	}
	out := picked
	for _, s := range full {
		if !strings.HasSuffix(s.Name, suffix) || have[s.Name] {
			continue
		}
		if allowed != nil && s.Source != "" && !allowed[s.Source] {
			continue
		}
		sToks := cleanQueryTokens(tokenSet(s.Name + " " + s.Description))
		if !anyOverlap(qToks, sToks) {
			continue
		}
		out = append(out, s)
		have[s.Name] = true
	}
	return out
}

// hasDetailIntent reports whether the query explicitly asks for one record's
// detail (or identifies it by id).
func hasDetailIntent(query string) bool {
	q := strings.ToLower(query)
	for _, p := range detailIntentPatterns {
		if strings.Contains(q, p) {
			return true
		}
	}
	if matched, _ := regexp.MatchString(`\bid\b\s*=?\s*\d+`, q); matched {
		return true
	}
	return false
}

// hasListIntent reports whether the query explicitly asks for a paged list.
func hasListIntent(query string) bool {
	q := strings.ToLower(query)
	for _, p := range listIntentPatterns {
		if strings.Contains(q, p) {
			return true
		}
	}
	// "前5个" / "第1页" numeric forms.
	if matched, _ := regexp.MatchString(`前\s*\d+\s*(个|条|名)`, q); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`第\s*\d+\s*页`, q); matched {
		return true
	}
	return false
}

// anyOverlap reports whether two token sets share a token.
func anyOverlap(a, b map[string]bool) bool {
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	for t := range small {
		if large[t] {
			return true
		}
	}
	return false
}

// preserveAuthTools forces a selected system's login/session primitives into
// the candidate set. After a backend returns 401 the model must be able to call
// *_login (establish a session) or *_me / current-session probes (check it),
// and those names never appear in the user query. Only systems in allowed (or
// every system when allowed is nil) contribute, and a short, conservative name
// match keeps the set tiny.
func preserveAuthTools(full []llm.ToolSpec, allowed map[string]bool, picked []llm.ToolSpec) []llm.ToolSpec {
	have := make(map[string]bool, len(picked))
	for _, s := range picked {
		have[s.Name] = true
	}
	out := picked
	for _, s := range full {
		if !isAuthSessionTool(s) || have[s.Name] {
			continue
		}
		if allowed != nil && s.Source != "" && !allowed[s.Source] {
			continue
		}
		out = append(out, s)
		have[s.Name] = true
	}
	return out
}

// isAuthSessionTool conservatively identifies login / current-session tools by
// name: an "*AuthController_*" operation restricted to login/me/session, or an
// operation explicitly ending in "_login". Register/setPassword/phoneLogin are
// excluded: they are not needed to recover an existing session.
func isAuthSessionTool(s llm.ToolSpec) bool {
	n := s.Name
	if strings.HasSuffix(n, "_login") {
		return true
	}
	idx := strings.Index(n, "AuthController_")
	if idx < 0 {
		return false
	}
	op := n[idx+len("AuthController_"):]
	return op == "login" || op == "me"
}

// connectorSources returns the distinct non-empty ToolSpec.Source ids, sorted.
func connectorSources(specs []llm.ToolSpec) []string {
	seen := make(map[string]bool)
	for _, s := range specs {
		if s.Source != "" {
			seen[s.Source] = true
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// validSystemNames drops system ids that were not offered and de-duplicates,
// preserving option order.
func validSystemNames(picked, options []string) []string {
	allowed := make(map[string]bool, len(options))
	for _, o := range options {
		allowed[o] = true
	}
	seen := make(map[string]bool, len(picked))
	out := make([]string, 0, len(picked))
	for _, s := range picked {
		if !allowed[s] || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// decideTrajectoryMaxRunes caps the compact execution trail appended to the
// decision probe. The trail is a cheap hint about where the plan is, not a
// replay of full tool payloads (those can be large), so each call and result
// is truncated hard.
const decideTrajectoryMaxRunes = 800

const decideTrajectoryItemRunes = 120

// buildCompactTrajectory renders the already-executed portion of the ReAct
// loop as a short, ordered list:
//
//	-> tool_a(args hint)
//	<- short result
//
// Only assistant tool calls and the corresponding tool messages are emitted;
// plain prose is skipped. The whole trail is capped to keep the probe cheap.
// On the first step there is no trail and it returns "".
func buildCompactTrajectory(messages []llm.Message, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	// Pre-pass: index every tool result. Results always appear AFTER the
	// assistant tool-call message, so a single forward pass would emit the
	// call before its result exists.
	resultByID := make(map[string]string)
	for _, m := range messages {
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			r := []rune(strings.TrimSpace(m.Content))
			if len(r) > decideTrajectoryItemRunes {
				r = r[:decideTrajectoryItemRunes]
			}
			resultByID[m.ToolCallID] = string(r)
		}
	}

	var lines []string
	for _, m := range messages {
		if m.Role != llm.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			lines = append(lines, "-> "+tc.Name+"("+compactArgs(tc.Arguments)+")")
			if res, ok := resultByID[tc.ID]; ok {
				lines = append(lines, "<- "+res)
			}
		}
	}

	var b strings.Builder
	for _, ln := range lines {
		if b.Len() > 0 && b.Len()+len(ln)+1 > maxRunes {
			break
		}
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// compactArgs renders a tool call's arguments as a short key=value hint so the
// decision model can see WHAT was queried (e.g. the SKU id) without carrying
// the full payload.
func compactArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(fmt.Sprintf("%v", args[k]))
		if r := []rune(v); len(r) > 40 {
			v = string(r[:40])
		}
		parts = append(parts, k+"="+v)
	}
	s := strings.Join(parts, ", ")
	if r := []rune(s); len(r) > decideTrajectoryItemRunes {
		s = string(r[:decideTrajectoryItemRunes])
	}
	return s
}

// protectedToolNames lists system tools that must remain callable regardless
// of the query. activate_skill is appended by specsForRun even when the user
// never says "activate a skill", so keyword matching would otherwise drop it
// and the model could no longer pull in a needed skill mid-run.
var protectedToolNames = map[string]bool{
	skill.ActivateToolName: true,
}

// preserveEssentialTools returns picked with protected system tools that must
// always survive re-appended if missing (activate_skill). It deliberately does
// NOT blanket-keep every Source="" tool: built-in workspace tools are routed
// via keyword prefilter (relevant -> included), while optional skill-registered
// tools that merely lack a connector (e.g. the mock-ticket "list_tickets") must
// not leak into every run. It never introduces a tool the run did not have.
func preserveEssentialTools(picked, full []llm.ToolSpec) []llm.ToolSpec {
	have := make(map[string]bool, len(picked))
	for _, s := range picked {
		have[s.Name] = true
	}
	out := picked
	for _, s := range full {
		if protectedToolNames[s.Name] && !have[s.Name] {
			out = append(out, s)
			have[s.Name] = true
		}
	}
	return out
}
