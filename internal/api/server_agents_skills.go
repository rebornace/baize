package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/workflow"
)

func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	var body struct {
		System      string   `json:"system"`
		Skills      []string `json:"skills"`
		ToolBinding string   `json:"tool_binding"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	binding, err := normalizeToolBinding(body.ToolBinding)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var warnings []string
	if s.SkillCatalog != nil {
		for _, sid := range body.Skills {
			p, ok := s.SkillCatalog.Get(sid)
			if !ok {
				continue
			}
			if missing := s.missingSkillTools(p.RequiresTools); len(missing) > 0 {
				writeError(w, http.StatusBadRequest, "skill_requires_tools",
					fmt.Sprintf("skill %q requires missing tools: %s", sid, strings.Join(missing, ", ")))
				return
			}
			if p.Domain != "" {
				warnings = append(warnings, fmt.Sprintf("domain_skill_default:%s:%s", sid, p.Domain))
			}
			if len(p.RequiresInput) > 0 {
				warnings = append(warnings, fmt.Sprintf("requires_input:%s:%s", sid, strings.Join(p.RequiresInput, ",")))
			}
		}
	}
	ag := store.Agent{ID: id, System: body.System, Skills: body.Skills, ToolBinding: binding}
	s.Store.UpsertAgent(ag)
	writeJSON(w, http.StatusOK, struct {
		store.Agent
		Warnings []string `json:"warnings,omitempty"`
	}{Agent: ag, Warnings: warnings})
}

func normalizeToolBinding(v string) (string, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "", store.ToolBindingFloor:
		return "", nil // omit floor in JSON; empty means floor
	case store.ToolBindingExclusive:
		return store.ToolBindingExclusive, nil
	default:
		return "", errors.New("tool_binding must be empty, floor, or exclusive")
	}
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	ag, err := s.Store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}
	writeJSON(w, http.StatusOK, ag)
}

type skillSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DescriptionEN string   `json:"description_en,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Tools         []string `json:"tools"`
	RequiresTools []string `json:"requires_tools,omitempty"`
	RequiresInput []string `json:"requires_input,omitempty"`
	MissingTools  []string `json:"missing_tools,omitempty"`
	Source        string   `json:"source"`
	EvalCaseCount int      `json:"eval_case_count"`
	HasWorkflow   bool     `json:"has_workflow,omitempty"`
}

func skillSummaryFrom(p skill.Package) skillSummary {
	tools := p.Tools
	if tools == nil {
		tools = []string{}
	}
	return skillSummary{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		DescriptionEN: p.DescriptionEN,
		Domain:        p.Domain,
		Tools:         tools,
		RequiresTools: p.RequiresTools,
		RequiresInput: p.RequiresInput,
		Source:        p.Source,
		EvalCaseCount: evalCaseCount(p),
		HasWorkflow:   p.Workflow != nil,
	}
}

func (s *Server) missingSkillTools(required []string) []string {
	if len(required) == 0 || s.Registry == nil {
		return nil
	}
	var missing []string
	for _, name := range required {
		if _, ok := s.Registry.Get(name); !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

// skillSummaryForLocale returns a summary whose Description field is already
// localized for the request locale (en / zh-CN). DescriptionEN is still
// included when present so clients can switch without another round-trip.
func skillSummaryForLocale(p skill.Package, locale string) skillSummary {
	sum := skillSummaryFrom(p)
	sum.Description = p.LocalizedDescription(locale)
	return sum
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	pkgs := s.SkillCatalog.List()
	out := make([]skillSummary, 0, len(pkgs))
	for _, p := range pkgs {
		sum := skillSummaryForLocale(p, locale)
		sum.MissingTools = s.missingSkillTools(p.RequiresTools)
		out = append(out, sum)
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	p, ok := s.SkillCatalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "skill not found")
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	sum := skillSummaryForLocale(p, locale)
	sum.MissingTools = s.missingSkillTools(p.RequiresTools)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":              sum.ID,
		"name":            sum.Name,
		"description":     sum.Description,
		"description_en":  sum.DescriptionEN,
		"domain":          sum.Domain,
		"tools":           sum.Tools,
		"requires_tools":  sum.RequiresTools,
		"requires_input":  sum.RequiresInput,
		"missing_tools":   sum.MissingTools,
		"has_workflow":    sum.HasWorkflow,
		"source":          sum.Source,
		"eval_case_count": sum.EvalCaseCount,
		"body":            p.LocalizedBody(locale),
	})
}

func (s *Server) handlePostSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid multipart form")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "file is required")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	var pkg skill.Package
	switch ext {
	case ".md":
		pkg, err = s.SkillCatalog.InstallMD(hdr.Filename, raw)
	case ".zip":
		pkg, err = s.SkillCatalog.InstallZip(raw)
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "file must be .md or .zip")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skillSummaryFrom(pkg))
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	if err := s.SkillCatalog.DeleteUser(id); err != nil {
		switch {
		case errors.Is(err, skill.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		case errors.Is(err, skill.ErrBuiltin):
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	for _, a := range s.Store.ListAgents() {
		next := make([]string, 0, len(a.Skills))
		changed := false
		for _, sid := range a.Skills {
			if sid == id {
				changed = true
				continue
			}
			next = append(next, sid)
		}
		if changed {
			a.Skills = next
			s.Store.UpsertAgent(a)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type skillWorkflowView struct {
	SkillID   string             `json:"skill_id"`
	Editable  bool               `json:"editable"`
	Workflow  *workflow.Workflow `json:"workflow,omitempty"`
	YAML      string             `json:"yaml,omitempty"`
	Warnings  []string           `json:"warnings,omitempty"`
	Source    string             `json:"source"`
	Missing   []string           `json:"missing_tools,omitempty"`
	CanCreate bool               `json:"can_create"`
}

func (s *Server) workflowView(p skill.Package) skillWorkflowView {
	view := skillWorkflowView{
		SkillID:   p.ID,
		Editable:  p.Source == skill.SourceUser,
		Workflow:  p.Workflow,
		Source:    p.Source,
		Missing:   s.missingSkillTools(p.RequiresTools),
		CanCreate: p.Source == skill.SourceUser,
		Warnings:  workflowWarnings(p),
	}
	if s.SkillCatalog != nil {
		if raw, ok := s.SkillCatalog.WorkflowYAML(p.ID); ok {
			view.YAML = raw
		}
	}
	return view
}

func workflowWarnings(p skill.Package) []string {
	var out []string
	if p.Domain != "" {
		out = append(out, "domain:"+p.Domain)
	}
	if len(p.RequiresInput) > 0 {
		out = append(out, "requires_input:"+strings.Join(p.RequiresInput, ","))
	}
	return out
}

func (s *Server) handleGetSkillWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	p, ok := s.SkillCatalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "skill not found")
		return
	}
	writeJSON(w, http.StatusOK, s.workflowView(p))
}

func (s *Server) handlePutSkillWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var wf workflow.Workflow
	if err := dec.Decode(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	wf.Name = id
	if err := wf.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	if err := workflow.ValidateReferences(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	if s.Registry != nil {
		for _, step := range wf.Steps {
			if _, ok := s.Registry.Get(step.Tool); !ok {
				writeError(w, http.StatusBadRequest, "invalid_workflow",
					fmt.Sprintf("step %q: unknown tool %q", step.ID, step.Tool))
				return
			}
		}
	}
	pkg, err := s.SkillCatalog.PutWorkflow(id, &wf)
	if err != nil {
		switch {
		case errors.Is(err, skill.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, s.workflowView(pkg))
}

func (s *Server) handlePutSkillWorkflowRaw(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	wf, err := workflow.Parse([]byte(body.YAML))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	if err := workflow.ValidateReferences(wf); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	if s.Registry != nil {
		for _, step := range wf.Steps {
			if _, ok := s.Registry.Get(step.Tool); !ok {
				writeError(w, http.StatusBadRequest, "invalid_workflow",
					fmt.Sprintf("step %q: unknown tool %q", step.ID, step.Tool))
				return
			}
		}
	}
	pkg, err := s.SkillCatalog.PutWorkflowRaw(id, body.YAML)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.workflowView(pkg))
}

type workflowPreviewStep struct {
	ID     string         `json:"id"`
	Tool   string         `json:"tool"`
	Status string         `json:"status"`
	Args   map[string]any `json:"args,omitempty"`
	Error  string         `json:"error,omitempty"`
}

func (s *Server) handlePreviewSkillWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	p, ok := s.SkillCatalog.Get(id)
	if !ok || p.Workflow == nil {
		writeError(w, http.StatusNotFound, "not_found", "skill workflow not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body struct {
		Input       map[string]any            `json:"input"`
		StepResults map[string]map[string]any `json:"step_results"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if body.Input == nil {
		body.Input = map[string]any{}
	}
	tree := map[string]any{"input": body.Input}
	out := make([]workflowPreviewStep, 0, len(p.Workflow.Steps))
	for _, step := range p.Workflow.Steps {
		item := workflowPreviewStep{ID: step.ID, Tool: step.Tool}
		if step.When != nil {
			ok, err := workflow.EvalCondition(step.When, tree)
			if err != nil {
				item.Status = "error"
				item.Error = err.Error()
				out = append(out, item)
				continue
			}
			if !ok {
				item.Status = "skipped"
				out = append(out, item)
				continue
			}
		}
		args, err := workflow.TryRenderArgs(step.Args, tree)
		if err != nil {
			item.Status = "error"
			item.Error = err.Error()
			out = append(out, item)
			continue
		}
		item.Status = "rendered"
		item.Args = args
		result := body.StepResults[step.ID]
		if result == nil {
			result = map[string]any{"ok": true}
		}
		workflow.SetResult(tree, step.ID, result)
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": out, "tree": tree})
}

func (s *Server) handleCloneSkillWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	pkg, err := s.SkillCatalog.CloneBuiltinToUser(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.workflowView(pkg))
}

func (s *Server) handleDeleteSkillWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	if err := s.SkillCatalog.DeleteWorkflow(id); err != nil {
		switch {
		case errors.Is(err, skill.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
